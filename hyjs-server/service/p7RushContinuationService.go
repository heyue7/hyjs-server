package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP7RushContinuation 保留原前三层目录，新增明确本地后继层的关闭目录。
func (s *GameService) InitializeP7RushContinuation(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7RushContinuationPolicy{}); err != nil {
		return err
	}
	policy := db.P7RushContinuationPolicy{ID: constUtil.P7RushContinuationPolicyID, Version: constUtil.P7RushContinuationVersion, Source: constUtil.P7RushContinuationSource, FirstFloor: 4, LastFloor: 6}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error; err != nil {
		return err
	}
	if err := tx.First(&policy, constUtil.P7RushContinuationPolicyID).Error; err != nil {
		return err
	}
	if err := p7ValidateRushContinuationPolicy(&policy); err != nil {
		return err
	}
	rows, err := p7RushContinuationCatalogue(policy.FirstFloor, policy.LastFloor, false)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&rows, 100).Error
}

func p7ValidateRushContinuationPolicy(policy *db.P7RushContinuationPolicy) error {
	if policy.ID != constUtil.P7RushContinuationPolicyID || policy.Version != constUtil.P7RushContinuationVersion || policy.Source != constUtil.P7RushContinuationSource || policy.FirstFloor != 4 || policy.LastFloor < 4 || policy.LastFloor > 1000 {
		return errors.New("本地试炼后继目录策略无效")
	}
	return nil
}

// p7RushContinuationCatalogue 从同一原NPC完整字节生成可审查的本地难度，保留原技能及实例。
func p7RushContinuationCatalogue(first, last int32, enabled bool) ([]db.P7RushStageRule, error) {
	if first < 4 || last < first || last > 1000 {
		return nil, errors.New("本地试炼后继楼层范围无效")
	}
	raw, err := hex.DecodeString(constUtil.P7SectionEnemyHex)
	if err != nil || p6Digest(raw) != constUtil.P7SectionEnemySHA {
		return nil, errors.New("本地试炼后继原NPC来源损坏")
	}
	base := &pb.BattleMonster{}
	if err = proto.Unmarshal(raw, base); err != nil || base.Inst == nil || len(base.Formation) != 1 || base.Formation[0].Prop == nil || base.Formation[0].Inst == nil {
		return nil, errors.New("本地试炼后继原NPC实例缺失")
	}
	var rows []db.P7RushStageRule
	for floor := first; floor <= last; floor++ {
		monster := proto.Clone(base).(*pb.BattleMonster)
		actor := monster.Formation[0]
		scale := 1 + float64(floor-3)*0.5
		actor.Prop.Att = proto.Float64(math.Floor(actor.Prop.GetAtt() * scale))
		actor.Prop.Hp = proto.Float64(math.Floor(actor.Prop.GetHp() * scale))
		actor.Prop.Pdef = proto.Float64(math.Floor(actor.Prop.GetPdef() * scale))
		actor.Prop.Mdef = proto.Float64(math.Floor(actor.Prop.GetMdef() * scale))
		actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp())
		enemy, err := proto.MarshalOptions{Deterministic: true}.Marshal(monster)
		if err != nil {
			return nil, err
		}
		condition := fmt.Sprintf("local-continuation-%d", floor-1)
		if floor == 4 {
			condition = "local-section-3"
		}
		for star := int32(1); star <= 3; star++ {
			rows = append(rows, db.P7RushStageRule{ID: fmt.Sprintf("local-continuation:%d:%d", floor, star), Kind: constUtil.P7RushTrial, TaskID: fmt.Sprintf("local-continuation-%d", floor), Floor: floor, Star: star, ConditionTaskID: condition, MinLevel: 22, Enabled: enabled, Scene: "map_grass_001", MaxRound: 40, Enemies: enemy, Reward: "point,10", Source: constUtil.P7RushContinuationSource})
		}
	}
	return rows, nil
}

// p7RushContinuationStageAvailable 同时验证发布目录与本轮前三层真实完整来源。
func (s *GameService) p7RushContinuationStageAvailable(tx *gorm.DB, state *db.P7RushState, stage db.P7RushStageRule) (bool, error) {
	if stage.Source != constUtil.P7RushContinuationSource {
		return true, nil
	}
	var policy db.P7RushContinuationPolicy
	if err := tx.First(&policy, constUtil.P7RushContinuationPolicyID).Error; err != nil {
		return false, err
	}
	if !policy.Enabled || !stage.Enabled {
		return false, nil
	}
	if err := p7ValidateRushContinuationPolicy(&policy); err != nil {
		return false, err
	}
	if stage.Floor < policy.FirstFloor || stage.Floor > policy.LastFloor || stage.Star < 1 || stage.Star > 3 {
		return false, nil
	}
	rows, err := p7RushContinuationCatalogue(stage.Floor, stage.Floor, true)
	if err != nil {
		return false, err
	}
	want := rows[stage.Star-1]
	a, err := json.Marshal(stage)
	if err != nil {
		return false, err
	}
	b, err := json.Marshal(want)
	if err != nil || !bytes.Equal(a, b) || !bytes.Equal(stage.Enemies, want.Enemies) {
		return false, errors.New("本地试炼后继楼层与发布目录冲突")
	}
	return s.p7CompletedLocalSectionResetAllowed(tx, state)
}

// p7RushContinuationTaskProjection 使用原Ready的实际point预览，不制造首通奖励。
func p7RushContinuationTaskProjection(stage db.P7RushStageRule) (*pb.RushTaskInfo, error) {
	if stage.Source != constUtil.P7RushContinuationSource && stage.Source != constUtil.P7SectionStageSource || stage.Reward != "point,10" || stage.FirstReward != "" {
		return nil, errors.New("本地试炼后继奖励展示冲突")
	}
	return &pb.RushTaskInfo{TaskId: proto.String(stage.TaskID), Star: proto.Int32(stage.Star), FightScore: proto.Float64(stage.RecommendedPower), IsPassed: proto.Bool(false), Point: proto.Int32(10), Coin: proto.Int32(0)}, nil
}

// p7PendingRushContinuation 恢复本地首组和后继的当前轮合法快照，不升级旧来源或别人的战斗权限。
func (s *GameService) p7PendingRushContinuation(tx *gorm.DB, state *db.P7RushState) (*pb.SCRushFightStart, *db.P7RushStageRule, bool, error) {
	if state.ActiveFight == "" {
		return nil, nil, false, nil
	}
	var fight db.P7RushFight
	if err := tx.First(&fight, "id=? AND player_id=? AND run_id=?", state.ActiveFight, state.PlayerID, state.RunID).Error; err != nil {
		return nil, nil, false, err
	}
	var stage db.P7RushStageRule
	if p6Digest(fight.Rule) != fight.RuleHash || json.Unmarshal(fight.Rule, &stage) != nil {
		return nil, nil, false, errors.New("试炼活动战斗规则快照损坏")
	}
	if stage.Source != constUtil.P7RushContinuationSource && stage.Source != constUtil.P7SectionStageSource {
		return nil, nil, false, nil
	}
	if fight.Diagnostic || fight.Kind != constUtil.P7RushTrial || fight.Status != constUtil.P7RushComputed || stage.Floor != state.Floor || stage.TaskID != fight.TaskID || p6Digest(fight.Start) != fight.StartHash {
		return nil, nil, true, errors.New("本地后继活动战斗归属冲突")
	}
	start := &pb.SCRushFightStart{}
	if err := proto.Unmarshal(fight.Start, start); err != nil {
		return nil, nil, true, err
	}
	outcome, scoped, err := p7ReadScopedRush(tx, &fight, nil)
	if err != nil {
		return nil, nil, true, err
	}
	if !scoped || outcome == nil || !outcome.Authoritative || start.GetFightId() != fight.ID || start.GetRushType() != fight.Kind || start.BattlePlayer.GetPlayerId() != state.PlayerID {
		return nil, nil, true, errors.New("本地后继活动战斗缺少完整冻结来源")
	}
	return start, &stage, true, nil
}

// p7PendingRushContinuationInfo 沿原Info字段恢复同一待结束战斗的入口。
func (s *GameService) p7PendingRushContinuationInfo(tx *gorm.DB, player *pb.Player, state *db.P7RushState, reset db.P7RushPolicy, now time.Time) (*pb.SCGetRushInfo, error) {
	free, err := s.p7VIP(player, "numPerk2")
	if err != nil {
		return nil, err
	}
	paid, err := s.p7VIP(player, "numPerk23")
	if err != nil {
		return nil, err
	}
	return &pb.SCGetRushInfo{CurFloor: proto.Int32(state.Floor), CurTotalStar: proto.Int32(state.TotalStars), ResetLeftTime: proto.Int32(max(0, free-state.FreeResetUsed)), DiamondRestLeftTime: proto.Int32(max(0, paid-state.PaidResetUsed)), DiamondRestCostNum: proto.Int32(reset.TrialResetCost), HighRecordFloor: proto.Int32(state.HighFloor), RecordTotalStar: proto.Int32(state.RecordStars), CurFloorSceneName: proto.String(constUtil.P7OriginalScopedFloorScene), WinKey: proto.Int32(0), WinValue: proto.Int32(0), IsMustReset: proto.Int32(0), IsRewardNeedGet: proto.Int32(0), IsAdditionNeedGet: proto.Int32(0), IsMazeTraderValid: proto.Int32(0)}, nil
}

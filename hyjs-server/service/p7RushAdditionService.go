package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP7RushAddition 仅创建关闭的本地策略，初始化不会给历史局补发机会。
func (s *GameService) InitializeP7RushAddition(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7RushAdditionPolicy{}, &db.P7RushAdditionOffer{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7RushAdditionPolicy{ID: constUtil.P7RushAdditionPolicyID, Version: constUtil.P7RushAdditionVersion, Source: constUtil.P7RushAdditionSource, ThreeOption: "peratt,0.03", SixOption: "perhp,0.06", NineOption: "perpdef,0.09"}).Error
}

// p7RushAdditionOption 对原客户端的属性,小数比例表示做有限本地校验。
func p7RushAdditionOption(raw string) (string, float64, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 2 || len(raw) > 64 {
		return "", 0, errors.New("试炼加成选项无效")
	}
	switch parts[0] {
	case "peratt", "perhp", "perpdef", "permdef":
	default:
		return "", 0, errors.New("试炼加成属性未支持")
	}
	rate, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > 1 {
		return "", 0, errors.New("试炼加成比例无效")
	}
	return parts[0], rate, nil
}

func p7ValidateRushAdditionPolicy(p *db.P7RushAdditionPolicy) error {
	if p.ID != constUtil.P7RushAdditionPolicyID || p.Version != constUtil.P7RushAdditionVersion || p.Source != constUtil.P7RushAdditionSource || p.PublishedAt <= 0 || p.PublishedAt > p.StartAt || p.EndAt <= p.StartAt {
		return errors.New("试炼加成发布契约无效")
	}
	seen := map[string]bool{}
	for _, option := range []string{p.ThreeOption, p.SixOption, p.NineOption} {
		key, _, err := p7RushAdditionOption(option)
		if err != nil {
			return err
		}
		if seen[key] {
			return errors.New("试炼加成三选项属性重复")
		}
		seen[key] = true
	}
	return nil
}

// p7CaptureRushAdditionOffer 与真实首层Start同行事务，冻结策略和完整目录。
func (s *GameService) p7CaptureRushAdditionOffer(tx *gorm.DB, fight *db.P7RushFight) error {
	if fight.Diagnostic || fight.Kind != constUtil.P7RushTrial {
		return nil
	}
	var stage db.P7RushStageRule
	if p6Digest(fight.Rule) != fight.RuleHash || json.Unmarshal(fight.Rule, &stage) != nil {
		return errors.New("试炼加成首层快照损坏")
	}
	if stage.Floor != 1 || stage.Source != constUtil.P7SectionStageSource {
		return nil
	}
	var existing int64
	if err := tx.Model(&db.P7RushAdditionOffer{}).Where("player_id=? AND run_id=?", fight.PlayerID, fight.RunID).Count(&existing).Error; err != nil || existing > 0 {
		return err
	}
	var policy db.P7RushAdditionPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, constUtil.P7RushAdditionPolicyID).Error; err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	if err := p7ValidateRushAdditionPolicy(&policy); err != nil {
		return err
	}
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", fight.ReplayID).Error; err != nil {
		return err
	}
	if record.CreatedAt < policy.StartAt || record.CreatedAt < policy.PublishedAt || record.CreatedAt >= policy.EndAt {
		return nil
	}
	if record.PlayerID != fight.PlayerID || record.SourceFight != fight.ID || record.Source != constUtil.P7RushReplaySource || p6Digest(fight.Start) != fight.StartHash {
		return errors.New("试炼加成首层归属冲突")
	}
	stages, err := p7SectionCatalogueFreeze(tx, &stage)
	if err != nil || stages == nil {
		return err
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return tx.Create(&db.P7RushAdditionOffer{PlayerID: fight.PlayerID, RunID: fight.RunID, FirstFightID: fight.ID, FirstStartHash: fight.StartHash, StartedAt: record.CreatedAt, Policy: raw, PolicyHash: p6Digest(raw), StageRules: stages, StageHash: p6Digest(stages)}).Error
}

func p7ReadRushAdditionOffer(tx *gorm.DB, state *db.P7RushState) (*db.P7RushAdditionOffer, *db.P7RushAdditionPolicy, error) {
	// 早期独立scoped fixture只建原战斗表，未迁移加成时保留无加成行为。
	if !tx.Migrator().HasTable(&db.P7RushAdditionOffer{}) {
		return nil, nil, nil
	}
	offer := &db.P7RushAdditionOffer{}
	err := tx.First(offer, "player_id=? AND run_id=?", state.PlayerID, state.RunID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	policy := &db.P7RushAdditionPolicy{}
	if p6Digest(offer.Policy) != offer.PolicyHash || p6Digest(offer.StageRules) != offer.StageHash || json.Unmarshal(offer.Policy, policy) != nil || p7ValidateRushAdditionPolicy(policy) != nil || !policy.Enabled || offer.StartedAt < policy.StartAt || offer.StartedAt < policy.PublishedAt || offer.StartedAt >= policy.EndAt {
		return nil, nil, errors.New("试炼加成冻结契约损坏")
	}
	var first db.P7RushFight
	if err = tx.First(&first, "id=? AND player_id=? AND run_id=?", offer.FirstFightID, state.PlayerID, state.RunID).Error; err != nil {
		return nil, nil, err
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", first.ReplayID).Error; err != nil {
		return nil, nil, err
	}
	if first.Diagnostic || first.Kind != constUtil.P7RushTrial || first.StartHash != offer.FirstStartHash || p6Digest(first.Start) != first.StartHash || record.CreatedAt != offer.StartedAt || record.PlayerID != state.PlayerID || record.SourceFight != first.ID || record.Source != constUtil.P7RushReplaySource {
		return nil, nil, errors.New("试炼加成冻结首层归属冲突")
	}
	if offer.Selected == "" {
		if offer.SpentStars != 0 || offer.SelectedAt != 0 || offer.SelectionStamp != 0 || offer.AcceptanceHash != "" {
			return nil, nil, errors.New("试炼加成未购存档损坏")
		}
	} else if !p7RushAdditionMatches(policy, offer.Selected, offer.SpentStars) || offer.SelectionStamp <= 0 || offer.SelectedAt < offer.StartedAt || offer.SelectedAt >= policy.EndAt || offer.AcceptanceHash == "" {
		return nil, nil, errors.New("试炼加成购买存档损坏")
	}
	return offer, policy, nil
}

func p7RushAdditionMatches(policy *db.P7RushAdditionPolicy, selected string, cost int32) bool {
	return cost == 3 && selected == policy.ThreeOption || cost == 6 && selected == policy.SixOption || cost == 9 && selected == policy.NineOption
}

// p7RushAdditionProgress 复用三个不同楼层的完整accepted来源，独立星钱包只扣SpentStars。
func p7RushAdditionProgress(tx *gorm.DB, offer *db.P7RushAdditionOffer, policy *db.P7RushAdditionPolicy) (int32, bool, string, error) {
	adapter := &db.P7SectionRewardOffer{PlayerID: offer.PlayerID, RunID: offer.RunID, FirstFightID: offer.FirstFightID, StartedAt: offer.StartedAt, StageRules: offer.StageRules}
	return p7SectionProgress(tx, adapter, &db.P7SectionRewardPolicy{SectionFrom: 1, SectionTo: 3, EndAt: policy.EndAt})
}

func p7RushAdditionProp(selected string) (*pb.BaseProp, error) {
	prop := &pb.BaseProp{}
	if selected == "" {
		return prop, nil
	}
	key, rate, err := p7RushAdditionOption(selected)
	if err != nil {
		return nil, err
	}
	switch key {
	case "peratt":
		prop.Peratt = proto.Float64(rate)
	case "perhp":
		prop.Perhp = proto.Float64(rate)
	case "perpdef":
		prop.Perpdef = proto.Float64(rate)
	case "permdef":
		prop.Permdef = proto.Float64(rate)
	}
	return prop, nil
}

// p7RushAdditionFlag 只在当前轮真实通关三层且仍可支付时提示原购买页面。
func (s *GameService) p7RushAdditionFlag(tx *gorm.DB, state *db.P7RushState, now time.Time) (int32, error) {
	offer, policy, err := p7ReadRushAdditionOffer(tx, state)
	if err != nil || offer == nil {
		return 0, err
	}
	if offer.Selected != "" || state.ActiveFight != "" || state.Floor < 4 || now.UnixMilli() >= policy.EndAt {
		return 0, nil
	}
	stars, complete, _, err := p7RushAdditionProgress(tx, offer, policy)
	if err != nil {
		return 0, err
	}
	if complete && stars >= 3 {
		return 1, nil
	}
	return 0, nil
}

// p7CompletedLocalSectionResetAllowed 恢复已完整接受三层的原重置入口。
// 冻结分段奖励来源可独立授权，当前加成策略无需开启；旧轮次与未证星数不授予资格。
func (s *GameService) p7CompletedLocalSectionResetAllowed(tx *gorm.DB, state *db.P7RushState) (bool, error) {
	if state.ActiveFight != "" || state.Floor < 4 {
		return false, nil
	}
	if tx.Migrator().HasTable(&db.P7SectionRewardOffer{}) {
		section, policy, err := s.p7ReadSectionOffer(tx, state)
		if err != nil {
			return false, err
		}
		if section != nil {
			stars, complete, _, err := p7SectionProgress(tx, section, policy)
			return complete && stars >= 3 && state.TotalStars >= stars, err
		}
	}
	addition, cfg, err := p7ReadRushAdditionOffer(tx, state)
	if err != nil || addition == nil {
		return false, err
	}
	stars, complete, _, err := p7RushAdditionProgress(tx, addition, cfg)
	return complete && stars >= 3 && state.TotalStars >= stars, err
}

// p7EarnedRushAdditionOnlyInfo 保留未发布第四层时的原加成、查看与重置入口。
// 返回页不补造第四层任务；三个真实胜利后的机会可以在原弹窗中完成。
func (s *GameService) p7EarnedRushAdditionOnlyInfo(tx *gorm.DB, player *pb.Player, state *db.P7RushState, reset db.P7RushPolicy, now time.Time) (*pb.SCGetRushInfo, bool, error) {
	offer, policy, err := p7ReadRushAdditionOffer(tx, state)
	if err != nil || offer == nil || state.ActiveFight != "" || state.Floor < 4 {
		return nil, false, err
	}
	stars, complete, _, err := p7RushAdditionProgress(tx, offer, policy)
	if err != nil || !complete || stars < 3 {
		return nil, false, err
	}
	free, err := s.p7VIP(player, "numPerk2")
	if err != nil {
		return nil, false, err
	}
	paid, err := s.p7VIP(player, "numPerk23")
	if err != nil {
		return nil, false, err
	}
	flag, err := s.p7RushAdditionFlag(tx, state, now)
	if err != nil {
		return nil, false, err
	}
	mustReset, maze, err := s.p7MazeTraderFlags(tx, state, now)
	if err != nil {
		return nil, false, err
	}
	return &pb.SCGetRushInfo{IsAdditionNeedGet: proto.Int32(flag), IsRewardNeedGet: proto.Int32(0), IsMustReset: proto.Int32(mustReset), IsMazeTraderValid: proto.Int32(maze), CurFloor: proto.Int32(state.Floor), CurTotalStar: proto.Int32(state.TotalStars), ResetLeftTime: proto.Int32(max(0, free-state.FreeResetUsed)), DiamondRestLeftTime: proto.Int32(max(0, paid-state.PaidResetUsed)), DiamondRestCostNum: proto.Int32(reset.TrialResetCost), HighRecordFloor: proto.Int32(state.HighFloor), RecordTotalStar: proto.Int32(state.RecordStars), CurFloorSceneName: proto.String(constUtil.P7OriginalScopedFloorScene), WinKey: proto.Int32(0), WinValue: proto.Int32(0)}, true, nil
}

// P7RushAddition 使用原三个请求，购买在profile行锁内一次记账且回执可冷重试。
func (s *GameService) P7RushAddition(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		purchase, buying := request.(*pb.CSRushAddition)
		if buying {
			old := &pb.SCRushAddition{}
			found, err := p7ReadReceipt(tx, id, constUtil.P7RushAdditionReceipt, stamp, request, old)
			if err != nil {
				return err
			}
			if found {
				response = old
				return nil
			}
		}
		state, err := p7State(tx, id, now)
		if err != nil {
			return err
		}
		offer, policy, err := p7ReadRushAdditionOffer(tx, state)
		if err != nil {
			return err
		}
		selected := ""
		if offer != nil {
			selected = offer.Selected
		}
		prop, err := p7RushAdditionProp(selected)
		if err != nil {
			return err
		}
		if _, viewing := request.(*pb.CSRushAdditionView); viewing {
			response = &pb.SCRushAdditionView{Prop: prop}
			return nil
		}
		if _, preview := request.(*pb.CSRushGetSectionAddition); !preview && !buying {
			return errors.New("试炼加成协议无效")
		}
		if offer == nil || buying && selected != "" || state.ActiveFight != "" || state.Floor < 4 || now.UnixMilli() < policy.StartAt || now.UnixMilli() >= policy.EndAt {
			return errors.New("本轮试炼加成尚未解锁或已经购买")
		}
		stars, complete, acceptance, err := p7RushAdditionProgress(tx, offer, policy)
		if err != nil {
			return err
		}
		if !complete || stars < 3 || state.TotalStars < stars {
			return errors.New("试炼加成缺少本轮真实通关星数")
		}
		if !buying {
			response = &pb.SCRushGetSectionAddition{StarResult3: proto.String(policy.ThreeOption), StarResult6: proto.String(policy.SixOption), StarResult9: proto.String(policy.NineOption), Prop: prop, CostStar: proto.Int32(stars - offer.SpentStars)}
			return nil
		}
		if !p7RushAdditionMatches(policy, purchase.GetSelect(), purchase.GetStar()) || purchase.GetStar() > stars {
			return errors.New("试炼加成选项或可用星数不匹配")
		}
		offer.Selected = purchase.GetSelect()
		offer.SpentStars = purchase.GetStar()
		offer.SelectedAt = now.UnixMilli()
		offer.SelectionStamp = stamp
		offer.AcceptanceHash = acceptance
		if err = tx.Save(offer).Error; err != nil {
			return err
		}
		prop, err = p7RushAdditionProp(offer.Selected)
		if err != nil {
			return err
		}
		response = &pb.SCRushAddition{Prop: prop}
		return p7SaveReceipt(tx, id, constUtil.P7RushAdditionReceipt, stamp, request, response)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// p7ApplyRushAddition 把已购比例展开进新快照；原引擎不消费BaseProp.per元数据。
// 原始profile/其他模式面板保持不动，精英与诊断调用不接此入口。
func (s *GameService) p7ApplyRushAddition(tx *gorm.DB, state *db.P7RushState, player *pb.BattlePlayer) (*pb.BattlePlayer, error) {
	offer, policy, err := p7ReadRushAdditionOffer(tx, state)
	if err != nil || offer == nil || offer.Selected == "" {
		return player, err
	}
	stars, complete, acceptance, err := p7RushAdditionProgress(tx, offer, policy)
	if err != nil {
		return nil, err
	}
	if !complete || stars < offer.SpentStars || acceptance != offer.AcceptanceHash {
		return nil, errors.New("试炼加成已购来源冲突")
	}
	return p7FoldRushAddition(player, offer.Selected)
}

func p7FoldRushAddition(player *pb.BattlePlayer, selected string) (*pb.BattlePlayer, error) {
	key, rate, err := p7RushAdditionOption(selected)
	if err != nil || player == nil {
		return nil, errors.New("试炼加成战斗输入无效")
	}
	result := proto.Clone(player).(*pb.BattlePlayer)
	for _, actor := range result.Formation {
		if actor == nil || actor.Prop == nil || actor.Inst == nil {
			return nil, errors.New("试炼加成角色实例缺失")
		}
		var value float64
		switch key {
		case "peratt":
			value = math.Floor(actor.Prop.GetAtt() * (1 + rate))
			actor.Prop.Att = proto.Float64(value)
		case "perhp":
			value = math.Floor(actor.Prop.GetHp() * (1 + rate))
			actor.Prop.Hp = proto.Float64(value)
			actor.Inst.CurHp = proto.Float64(math.Floor(actor.Inst.GetCurHp() * (1 + rate)))
		case "perpdef":
			value = math.Floor(actor.Prop.GetPdef() * (1 + rate))
			actor.Prop.Pdef = proto.Float64(value)
		case "permdef":
			value = math.Floor(actor.Prop.GetMdef() * (1 + rate))
			actor.Prop.Mdef = proto.Float64(value)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.IsNaN(actor.Inst.GetCurHp()) || math.IsInf(actor.Inst.GetCurHp(), 0) || actor.Inst.GetCurHp() < 0 || actor.Inst.GetCurHp() > actor.Prop.GetHp() {
			return nil, errors.New("试炼加成展开属性无效")
		}
	}
	return result, nil
}

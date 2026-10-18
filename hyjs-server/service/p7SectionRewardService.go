package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func (s *GameService) InitializeP7SectionRewards(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7SectionRewardPolicy{}, &db.P7SectionRewardOffer{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7SectionRewardPolicy{ID: constUtil.P7SectionRewardPolicyID, Version: "local-first-section-v1", Source: constUtil.P7SectionRewardSource, SectionFrom: 1, SectionTo: 3, ThreeReward: "item_001,1", SixReward: "item_001,2", NineReward: "item_001,3"}).Error; err != nil {
		return err
	}
	rows, err := p7SectionCatalogue(false)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// p7SectionCatalogue preserves the full original-observed NPC. These nine rows
// are explicitly local floor/star choices; initialization never enables a row.
func p7SectionCatalogue(enabled bool) ([]db.P7RushStageRule, error) {
	enemy, err := hex.DecodeString(constUtil.P7SectionEnemyHex)
	if err != nil || p6Digest(enemy) != constUtil.P7SectionEnemySHA {
		return nil, errors.New("本地试炼原NPC来源校验失败")
	}
	rows := []db.P7RushStageRule{}
	for floor := int32(1); floor <= 3; floor++ {
		for star := int32(1); star <= 3; star++ {
			rows = append(rows, db.P7RushStageRule{ID: fmt.Sprintf("local-section:%d:%d", floor, star), TaskID: fmt.Sprintf("local-section-%d", floor), Kind: constUtil.P7RushTrial, Floor: floor, Star: star, MinLevel: 22, Enabled: enabled, Scene: "map_grass_001", MaxRound: 40, Enemies: enemy, Reward: "point,10", Source: constUtil.P7SectionStageSource})
		}
	}
	return rows, nil
}

// Freeze only the nine explicitly published local choices, retaining the full
// rule bytes. Subsequent source edits cannot grant this earlier section offer.
func p7SectionCatalogueFreeze(tx *gorm.DB, first *db.P7RushStageRule) ([]byte, error) {
	expected, err := p7SectionCatalogue(true)
	if err != nil {
		return nil, err
	}
	var rows []db.P7RushStageRule
	if err = tx.Where("id IN ?", []string{"local-section:1:1", "local-section:1:2", "local-section:1:3", "local-section:2:1", "local-section:2:2", "local-section:2:3", "local-section:3:1", "local-section:3:2", "local-section:3:3"}).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(expected) {
		return nil, errors.New("本地试炼前三层目录不完整")
	}
	found := false
	for i := range rows {
		a, _ := json.Marshal(rows[i])
		b, _ := json.Marshal(expected[i])
		if string(a) != string(b) {
			return nil, errors.New("本地试炼前三层配置未按完整目录发布")
		}
		if rows[i].ID == first.ID {
			r, _ := json.Marshal(first)
			if string(a) != string(r) {
				return nil, errors.New("本地试炼首层不属于发布目录")
			}
			found = true
		}
	}
	if !found {
		return nil, nil
	}
	return json.Marshal(rows)
}

func (s *GameService) p7ValidateSectionPolicy(policy *db.P7SectionRewardPolicy) error {
	if policy.ID != constUtil.P7SectionRewardPolicyID || policy.Version == "" || len(policy.Version) > 64 || policy.Source != constUtil.P7SectionRewardSource || policy.SectionFrom != 1 || policy.SectionTo != 3 || policy.PublishedAt <= 0 || policy.PublishedAt > policy.StartAt || policy.EndAt <= policy.StartAt {
		return errors.New("试炼分段奖励预发布规则无效")
	}
	for _, reward := range []string{policy.ThreeReward, policy.SixReward, policy.NineReward} {
		resources, err := p1RewardResources(reward)
		if err != nil || len(resources) == 0 || len(resources) > 8 {
			return errors.New("试炼星档奖励无效")
		}
		for _, item := range resources {
			key := item.GetId()
			if key != "coin" && key != "diamond" && key != "point" && !strings.HasPrefix(key, "item_") {
				return errors.New("本地试炼星档只投放已支持的基础资源和道具")
			}
		}
		if ok, err := s.openServerRewardSupported(reward); err != nil || !ok {
			return errors.New("试炼星档资源用途未恢复")
		}
	}
	return nil
}

// p7CaptureSectionOffer is called only after a real paid Trial Start was created.
// It never promotes diagnostic sources and never backfills earlier winning floors.
func (s *GameService) p7CaptureSectionOffer(tx *gorm.DB, fight *db.P7RushFight) error {
	if fight.Diagnostic || fight.Kind != constUtil.P7RushTrial {
		return nil
	}
	var cfg db.P7RushStageRule
	if p6Digest(fight.Rule) != fight.RuleHash || json.Unmarshal(fight.Rule, &cfg) != nil {
		return errors.New("试炼分段首层快照无效")
	}
	if cfg.Floor != 1 || cfg.Source != constUtil.P7SectionStageSource {
		return nil
	}
	var count int64
	if err := tx.Model(&db.P7SectionRewardOffer{}).Where("player_id=? AND run_id=?", fight.PlayerID, fight.RunID).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	var policy db.P7SectionRewardPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, constUtil.P7SectionRewardPolicyID).Error; err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	if err := s.p7ValidateSectionPolicy(&policy); err != nil {
		return err
	}
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", fight.ReplayID).Error; err != nil {
		return err
	}
	if record.PlayerID != fight.PlayerID || record.Source != constUtil.P7RushReplaySource || record.SourceFight != fight.ID || p6Digest(fight.Start) != fight.StartHash {
		return errors.New("试炼分段首层来源冲突")
	}
	if record.CreatedAt < policy.StartAt || record.CreatedAt < policy.PublishedAt || record.CreatedAt >= policy.EndAt {
		return nil
	}
	stages, err := p7SectionCatalogueFreeze(tx, &cfg)
	if err != nil {
		return err
	}
	if stages == nil {
		return nil
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return tx.Create(&db.P7SectionRewardOffer{PlayerID: fight.PlayerID, RunID: fight.RunID, FirstFightID: fight.ID, FirstStartHash: fight.StartHash, StartedAt: record.CreatedAt, StageRules: stages, Policy: raw, PolicyHash: p6Digest(raw)}).Error
}

func (s *GameService) p7ReadSectionOffer(tx *gorm.DB, state *db.P7RushState) (*db.P7SectionRewardOffer, *db.P7SectionRewardPolicy, error) {
	offer := &db.P7SectionRewardOffer{}
	err := tx.First(offer, "player_id=? AND run_id=?", state.PlayerID, state.RunID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	policy := &db.P7SectionRewardPolicy{}
	if p6Digest(offer.Policy) != offer.PolicyHash || json.Unmarshal(offer.Policy, policy) != nil || s.p7ValidateSectionPolicy(policy) != nil {
		return nil, nil, errors.New("试炼分段冻结奖励损坏")
	}
	var first db.P7RushFight
	if err = tx.First(&first, "id=? AND player_id=? AND run_id=?", offer.FirstFightID, state.PlayerID, state.RunID).Error; err != nil {
		return nil, nil, err
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", first.ReplayID).Error; err != nil {
		return nil, nil, err
	}
	if first.Kind != constUtil.P7RushTrial || first.Diagnostic || first.StartHash != offer.FirstStartHash || p6Digest(first.Start) != first.StartHash || record.CreatedAt != offer.StartedAt || record.PlayerID != state.PlayerID || record.SourceFight != first.ID || record.Source != constUtil.P7RushReplaySource || offer.StartedAt < policy.StartAt || offer.StartedAt < policy.PublishedAt || offer.StartedAt >= policy.EndAt {
		return nil, nil, errors.New("试炼分段首层冻结归属冲突")
	}
	return offer, policy, nil
}

// p7SectionProgress uses unique actual accepted winning floors from this run.
// The complete original End/P6 witness is rechecked without replaying computation.
func p7SectionProgress(tx *gorm.DB, offer *db.P7SectionRewardOffer, policy *db.P7SectionRewardPolicy) (int32, bool, string, error) {
	var rows []db.P7RushFight
	if err := tx.Where("player_id=? AND run_id=? AND kind=? AND status=? AND diagnostic=?", offer.PlayerID, offer.RunID, constUtil.P7RushTrial, constUtil.P7RushSettled, false).Find(&rows).Error; err != nil {
		return 0, false, "", err
	}
	var catalogue []db.P7RushStageRule
	if json.Unmarshal(offer.StageRules, &catalogue) != nil || len(catalogue) != 9 {
		return 0, false, "", errors.New("试炼冻结目录损坏")
	}
	allowed := map[string]string{}
	for _, r := range catalogue {
		raw, _ := json.Marshal(r)
		allowed[r.ID] = p6Digest(raw)
	}
	proofs := map[int32]map[string]any{}
	stars := int32(0)
	for i := range rows {
		fight := &rows[i]
		var rule db.P7RushStageRule
		if p6Digest(fight.Rule) != fight.RuleHash || json.Unmarshal(fight.Rule, &rule) != nil {
			return 0, false, "", errors.New("试炼分段任务快照损坏")
		}
		if rule.Floor < policy.SectionFrom || rule.Floor > policy.SectionTo {
			continue
		}
		if allowed[rule.ID] != fight.RuleHash {
			return 0, false, "", errors.New("试炼结算不属于首层冻结目录")
		}
		end := &pb.CSRushFightEnd{}
		result := &pb.SCRushFightEnd{}
		if proto.Unmarshal(fight.EndRequest, end) != nil || proto.Unmarshal(fight.Result, result) != nil || end.GetFightId() != fight.ID {
			return 0, false, "", errors.New("试炼分段结算回执损坏")
		}
		if result.GetResult() != 1 {
			continue
		}
		outcome, scoped, err := p7ReadScopedRush(tx, fight, end)
		if err != nil {
			return 0, false, "", err
		}
		if !scoped || outcome == nil || !outcome.Authoritative || outcome.Winner != 0 || end.GetResult() != 1 || rule.Kind != constUtil.P7RushTrial || rule.Star < 1 || rule.Star > 3 || result.GetStar() != rule.Star {
			return 0, false, "", errors.New("试炼分段胜利缺少完整独立接受来源")
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", fight.ReplayID).Error; err != nil {
			return 0, false, "", err
		}
		if record.CreatedAt < offer.StartedAt || record.CreatedAt >= policy.EndAt || record.PlayerID != offer.PlayerID || record.SourceFight != fight.ID {
			return 0, false, "", errors.New("试炼分段战斗不属于冻结窗口")
		}
		if rule.Floor == policy.SectionFrom && fight.ID != offer.FirstFightID {
			return 0, false, "", errors.New("试炼分段首层来源不一致")
		}
		if _, duplicate := proofs[rule.Floor]; duplicate {
			return 0, false, "", errors.New("本轮同层存在重复获胜来源")
		}
		proofs[rule.Floor] = map[string]any{"floor": rule.Floor, "star": rule.Star, "fight": fight.ID, "start": fight.StartHash, "rule": fight.RuleHash, "result": p6Digest(fight.Result), "end": p6Digest(fight.EndRequest), "proof": outcome.CapabilityHash, "record": record.ID, "outcome": record.OutcomeHash}
		stars += rule.Star
	}
	ordered := []map[string]any{}
	for floor := policy.SectionFrom; floor <= policy.SectionTo; floor++ {
		if proof, ok := proofs[floor]; ok {
			ordered = append(ordered, proof)
		}
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		return 0, false, "", err
	}
	return stars, len(ordered) == int(policy.SectionTo-policy.SectionFrom+1), p6Digest(raw), nil
}

func (s *GameService) p7SectionRewardFlag(tx *gorm.DB, state *db.P7RushState, now time.Time) (int32, error) {
	offer, policy, err := s.p7ReadSectionOffer(tx, state)
	if err != nil || offer == nil {
		return 0, err
	}
	if offer.ClaimedAt > 0 || state.ActiveFight != "" || now.UnixMilli() >= policy.EndAt {
		return 0, nil
	}
	stars, complete, _, err := p7SectionProgress(tx, offer, policy)
	if err != nil {
		return 0, err
	}
	if complete && stars >= 3 && state.Floor > policy.SectionTo {
		return 1, nil
	}
	return 0, nil
}

// P7SectionReward retains the original empty preview/claim protocol. Its three
// repeated resource lists display the fixed local version; claim awards only the
// highest achieved tier after all three real floors, as explicitly declared local.
func (s *GameService) P7SectionReward(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		claim := false
		switch request.(type) {
		case *pb.CSRushRewardPreview:
		case *pb.CSRushGetSectionReward:
			claim = true
		default:
			return errors.New("试炼分段奖励协议无效")
		}
		if claim {
			old := &pb.SCRushGetSectionReward{}
			found, err := p7ReadReceipt(tx, id, constUtil.P7SectionRewardReceipt, stamp, request, old)
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
		offer, policy, err := s.p7ReadSectionOffer(tx, state)
		if err != nil {
			return err
		}
		stars := int32(0)
		complete := false
		acceptance := ""
		if offer != nil {
			stars, complete, acceptance, err = p7SectionProgress(tx, offer, policy)
			if err != nil {
				return err
			}
		} else {
			policy = &db.P7SectionRewardPolicy{}
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(policy, constUtil.P7SectionRewardPolicyID).Error; err != nil {
				return err
			}
			if !policy.Enabled || state.Floor != 1 || state.ActiveFight != "" {
				return errors.New("本轮没有预发布的试炼分段奖励")
			}
			if err = s.p7ValidateSectionPolicy(policy); err != nil {
				return err
			}
		}
		if now.UnixMilli() < policy.StartAt || now.UnixMilli() >= policy.EndAt {
			return errors.New("试炼分段奖励窗口已关闭")
		}
		if !claim {
			three, err := p1RewardResources(policy.ThreeReward)
			if err != nil {
				return err
			}
			six, err := p1RewardResources(policy.SixReward)
			if err != nil {
				return err
			}
			nine, err := p1RewardResources(policy.NineReward)
			if err != nil {
				return err
			}
			response = &pb.SCRushRewardPreview{SectionStar: proto.Int32(stars), SectionFrom: proto.Int32(policy.SectionFrom), SectionTo: proto.Int32(policy.SectionTo), ThreeStarReward: three, SixStarReward: six, NineStarReward: nine}
			return nil
		}
		if offer == nil || offer.ClaimedAt > 0 || state.ActiveFight != "" || !complete || stars < 3 || state.Floor <= policy.SectionTo {
			return errors.New("本轮试炼分段尚未完成或已经领奖")
		}
		reward := policy.ThreeReward
		if stars >= 9 {
			reward = policy.NineReward
		} else if stars >= 6 {
			reward = policy.SixReward
		}
		gain, err := s.GrantFixedReward(player, game, reward)
		if err != nil {
			return err
		}
		offer.ClaimedAt = now.UnixMilli()
		offer.ClaimStamp = stamp
		offer.ClaimedStars = stars
		offer.AcceptanceHash = acceptance
		if err = tx.Save(offer).Error; err != nil {
			return err
		}
		response = &pb.SCRushGetSectionReward{Gain: gain}
		return p7SaveReceipt(tx, id, constUtil.P7SectionRewardReceipt, stamp, request, response)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// p7EarnedSectionOnlyInfo exposes an already earned reward when the next floor
// has no published content. It supplies no enemies and never opens that battle.
func (s *GameService) p7EarnedSectionOnlyInfo(tx *gorm.DB, player *pb.Player, state *db.P7RushState, reset db.P7RushPolicy, now time.Time) (*pb.SCGetRushInfo, bool, error) {
	offer, policy, err := s.p7ReadSectionOffer(tx, state)
	if err != nil || offer == nil {
		return nil, false, err
	}
	stars, complete, _, err := p7SectionProgress(tx, offer, policy)
	if err != nil || !complete || stars < 3 || state.Floor <= policy.SectionTo || state.ActiveFight != "" || now.UnixMilli() >= policy.EndAt {
		return nil, false, err
	}
	ready := int32(0)
	if offer.ClaimedAt == 0 {
		ready = 1
	}
	free, err := s.p7VIP(player, "numPerk2")
	if err != nil {
		return nil, false, err
	}
	paid, err := s.p7VIP(player, "numPerk23")
	if err != nil {
		return nil, false, err
	}
	mustReset, mazeValid, err := s.p7MazeTraderFlags(tx, state, now)
	if err != nil {
		return nil, false, err
	}
	additionReady, err := s.p7RushAdditionFlag(tx, state, now)
	if err != nil {
		return nil, false, err
	}
	return &pb.SCGetRushInfo{IsAdditionNeedGet: proto.Int32(additionReady), CurFloor: proto.Int32(state.Floor), CurTotalStar: proto.Int32(state.TotalStars), ResetLeftTime: proto.Int32(max(0, free-state.FreeResetUsed)), DiamondRestLeftTime: proto.Int32(max(0, paid-state.PaidResetUsed)), DiamondRestCostNum: proto.Int32(reset.TrialResetCost), HighRecordFloor: proto.Int32(state.HighFloor), RecordTotalStar: proto.Int32(state.RecordStars), CurFloorSceneName: proto.String(constUtil.P7OriginalScopedFloorScene), WinKey: proto.Int32(0), WinValue: proto.Int32(0), IsRewardNeedGet: proto.Int32(ready), IsMustReset: proto.Int32(mustReset), IsMazeTraderValid: proto.Int32(mazeValid)}, true, nil
}

// The original Ready page displays the per-floor coin/point preview separately
// from the section reward. The new local catalogue projects its real point10.
func p7SectionTaskProjection(stage db.P7RushStageRule) (*pb.RushTaskInfo, error) {
	if stage.Source != constUtil.P7SectionStageSource || stage.Reward != "point,10" || stage.FirstReward != "" {
		return nil, errors.New("本地试炼目录展示与实际奖励不一致")
	}
	return &pb.RushTaskInfo{TaskId: proto.String(stage.TaskID), Star: proto.Int32(stage.Star), FightScore: proto.Float64(stage.RecommendedPower), IsPassed: proto.Bool(false), Point: proto.Int32(10), Coin: proto.Int32(0)}, nil
}

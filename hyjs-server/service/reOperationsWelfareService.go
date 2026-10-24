package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type reOperationsWelfareTier struct {
	Cost   int64  `json:"cost,omitempty"`
	Reward string `json:"reward"`
}
type reOperationsWelfareRules struct {
	DailySpend  int64                     `json:"dailySpend,omitempty"`
	Tiers       []reOperationsWelfareTier `json:"tiers"`
	FinalReward string                    `json:"finalReward,omitempty"`
}
type reOperationsWelfareProgress struct {
	CatUses      map[string]int32 `json:"catUses,omitempty"`
	Claimed      map[int]bool     `json:"claimed,omitempty"`
	ClaimDays    map[string]bool  `json:"claimDays,omitempty"`
	FinalClaimed bool             `json:"finalClaimed,omitempty"`
	LastGet      int64            `json:"lastGet,omitempty"`
}

func reOperationsWelfareForms() []model.AdminActivityForm {
	cat := model.AdminActivityForm{Mode: "fortune-cat", TemplateID: "fortune-cat-original", Name: "藏宝箱", Source: constUtil.ReOperationsWelfareSource, RepeatPolicy: "repeat", Products: []model.AdminActivityProduct{}, Prizes: []model.AdminActivityPrize{}, Milestones: []model.AdminActivityMilestone{}}
	for _, cost := range []int64{100, 300, 800, 1500, 3000} {
		cat.Milestones = append(cat.Milestones, model.AdminActivityMilestone{Goal: cost, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: cost + max(int64(1), cost/20)}}})
	}
	continued := model.AdminActivityForm{Mode: "continued-spend", TemplateID: "continued-spend-original", Name: "连续消费", Source: constUtil.ReOperationsWelfareSource, RepeatPolicy: "repeat", Products: []model.AdminActivityProduct{}, Prizes: []model.AdminActivityPrize{}, Milestones: []model.AdminActivityMilestone{}}
	continued.Options.DailySpend = 3000
	continued.Options.FinalRewards = []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 9000}}
	for day := int64(1); day <= 7; day++ {
		continued.Milestones = append(continued.Milestones, model.AdminActivityMilestone{Goal: day, Rewards: []model.AdminResourceAmount{{ResourceID: "diamond", Amount: day * 900}}})
	}
	return []model.AdminActivityForm{cat, continued}
}

func (s *GameService) validateReOperationsWelfare(cfg db.P4WelfareRule) error {
	var rules reOperationsWelfareRules
	if cfg.ID == "" || len(cfg.ID) > 64 || cfg.StartAt <= 0 || cfg.StartAt >= cfg.EndAt || cfg.ClaimEndAt < cfg.EndAt || cfg.Source == "" || json.Unmarshal(cfg.Rules, &rules) != nil {
		return constUtil.ErrAdminConfiguration
	}
	switch cfg.Kind {
	case constUtil.ReWelfareCatKind:
		if len(rules.Tiers) != 5 || cfg.ClaimEndAt != cfg.EndAt {
			return constUtil.ErrAdminConfiguration
		}
		for _, tier := range rules.Tiers {
			res, err := p1RewardResources(tier.Reward)
			// 接受旧冻结行供历史读取；新的发奖投影统一转换为金币。
			if err != nil || tier.Cost < 1 || tier.Cost > math.MaxInt32 || len(res) != 1 || (res[0].GetId() != "coin" && res[0].GetId() != "diamond") || res[0].GetNum() < 1 {
				return constUtil.ErrAdminConfiguration
			}
		}
	case constUtil.ReWelfareContinuedKind:
		if len(rules.Tiers) != 7 || rules.DailySpend < 1 || rules.DailySpend > math.MaxInt32 {
			return constUtil.ErrAdminConfiguration
		}
		for _, tier := range rules.Tiers {
			ok, err := s.openServerRewardSupported(tier.Reward)
			if err != nil || !ok {
				return constUtil.ErrAdminConfiguration
			}
		}
		ok, err := s.openServerRewardSupported(rules.FinalReward)
		if err != nil || !ok {
			return constUtil.ErrAdminConfiguration
		}
	default:
		return constUtil.ErrAdminConfiguration
	}
	return nil
}

// adminPersistReOperationsWelfare adapts the normal milestone editor to the two original request families.
// Frozen rows are protected by the existing typed-activity transaction before this function is reached.
func (s *GameService) adminPersistReOperationsWelfare(tx *gorm.DB, key string, form model.AdminActivityForm, enabled, exists bool) error {
	kind := constUtil.ReWelfareCatKind
	if form.Mode == "continued-spend" {
		kind = constUtil.ReWelfareContinuedKind
	}
	rules := reOperationsWelfareRules{DailySpend: form.Options.DailySpend, Tiers: []reOperationsWelfareTier{}}
	for i, m := range form.Milestones {
		if len(m.Options) > 0 || kind == constUtil.ReWelfareContinuedKind && m.Goal != int64(i+1) {
			return constUtil.ErrAdminInput
		}
		reward, err := s.adminTypedResourceSpec(m.Rewards, !enabled)
		if err != nil {
			return err
		}
		tier := reOperationsWelfareTier{Reward: reward}
		if kind == constUtil.ReWelfareCatKind {
			tier.Cost = m.Goal
			if len(m.Rewards) != 1 || m.Rewards[0].ResourceID != "coin" {
				return constUtil.ErrAdminInput
			}
		}
		rules.Tiers = append(rules.Tiers, tier)
	}
	var err error
	rules.FinalReward, err = s.adminTypedResourceSpec(form.Options.FinalRewards, kind == constUtil.ReWelfareCatKind || !enabled)
	if err != nil {
		return err
	}
	cfg := db.P4WelfareRule{ID: key, Kind: kind, Enabled: enabled, StartAt: form.StartAt, EndAt: form.EndAt, ClaimEndAt: form.ClaimEndAt, Rules: adminJSON(rules), Source: form.Source}
	if enabled {
		if err = s.validateReOperationsWelfare(cfg); err != nil {
			return err
		}
	}
	if exists {
		return tx.Save(&cfg).Error
	}
	return tx.Create(&cfg).Error
}

func (s *GameService) adminProjectReOperationsWelfare(cfg db.P4WelfareRule, form *model.AdminActivityForm) error {
	var rules reOperationsWelfareRules
	if json.Unmarshal(cfg.Rules, &rules) != nil {
		return constUtil.ErrAdminConfiguration
	}
	if cfg.Kind == constUtil.ReWelfareCatKind {
		if err := reTreasureCoinRewards(&rules); err != nil {
			return err
		}
	}
	form.Options.DailySpend = rules.DailySpend
	form.Options.FinalRewards = adminTypedResources(rules.FinalReward)
	form.Milestones = []model.AdminActivityMilestone{}
	for i, tier := range rules.Tiers {
		goal := int64(i + 1)
		if cfg.Kind == constUtil.ReWelfareCatKind {
			goal = tier.Cost
		}
		form.Milestones = append(form.Milestones, model.AdminActivityMilestone{Goal: goal, Rewards: adminTypedResources(tier.Reward)})
	}
	return nil
}

// reOperationsWelfareRuleKey gives role schedules precedence for the no-ID cat protocol.
// Explicit continued IDs are checked again under the profile lock, including realm/role ownership.
func reOperationsWelfareRuleKey(tx *gorm.DB, kind string, now time.Time) (string, error) {
	key, err := AdminCurrentCharacterWelfareRuleKey(tx, kind, now.UnixMilli())
	if err != nil || key != "" {
		return key, err
	}
	return AdminCurrentWelfareRuleKey(tx, kind, now.UnixMilli(), "")
}

func (s *GameService) loadReOperationsWelfare(tx *gorm.DB, owner int64, key, kind string, now time.Time) (*db.P4WelfareState, db.P4WelfareRule, reOperationsWelfareRules, *reOperationsWelfareProgress, error) {
	return s.loadReOperationsWelfareQuote(tx, owner, key, kind, now, false)
}

// Refresh only the next cat quote; existing progress and immutable receipts remain untouched.
func (s *GameService) loadReOperationsWelfareQuote(tx *gorm.DB, owner int64, key, kind string, now time.Time, refresh bool) (*db.P4WelfareState, db.P4WelfareRule, reOperationsWelfareRules, *reOperationsWelfareProgress, error) {
	var cfg db.P4WelfareRule
	var rules reOperationsWelfareRules
	state := &reOperationsWelfareProgress{CatUses: map[string]int32{}, Claimed: map[int]bool{}, ClaimDays: map[string]bool{}}
	row := &db.P4WelfareState{PlayerID: owner, RuleID: key}
	allowed, err := localPlayActivityVisible(tx, key)
	if err != nil {
		return nil, cfg, rules, nil, err
	}
	if !allowed {
		return nil, cfg, rules, nil, constUtil.ErrP4WelfareNotOpen
	}
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, "id=? AND kind=? AND enabled=?", key, kind, true).Error; err != nil {
		return nil, cfg, rules, nil, err
	}
	refresh = refresh && kind == constUtil.ReWelfareCatKind
	if len(cfg.Frozen) > 0 && !refresh {
		if json.Unmarshal(cfg.Frozen, &cfg) != nil {
			return nil, cfg, rules, nil, constUtil.ErrAdminConfiguration
		}
	} else if !refresh {
		if err = s.validateReOperationsWelfare(cfg); err != nil {
			return nil, cfg, rules, nil, err
		}
		if err = tx.Model(&db.P4WelfareRule{}).Where("id=?", key).Update("frozen", adminJSON(cfg)).Error; err != nil {
			return nil, cfg, rules, nil, err
		}
	}
	err = tx.First(row, "player_id=? AND rule_id=?", owner, key).Error
	if err == nil {
		if (!refresh && json.Unmarshal(row.Snapshot, &cfg) != nil) || json.Unmarshal(row.Payload, state) != nil {
			return nil, cfg, rules, nil, constUtil.ErrAdminConfiguration
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, cfg, rules, nil, err
	}
	if cfg.ID != key || cfg.Kind != kind || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.ClaimEndAt {
		return nil, cfg, rules, nil, constUtil.ErrP4WelfareNotOpen
	}
	if err = s.validateReOperationsWelfare(cfg); err != nil {
		return nil, cfg, rules, nil, err
	}
	if json.Unmarshal(cfg.Rules, &rules) != nil {
		return nil, cfg, rules, nil, constUtil.ErrAdminConfiguration
	}
	if kind == constUtil.ReWelfareCatKind {
		if err := reTreasureCoinRewards(&rules); err != nil {
			return nil, cfg, rules, nil, err
		}
	}
	if state.CatUses == nil {
		state.CatUses = map[string]int32{}
	}
	if state.Claimed == nil {
		state.Claimed = map[int]bool{}
	}
	if state.ClaimDays == nil {
		state.ClaimDays = map[string]bool{}
	}
	for _, count := range state.CatUses {
		if count < 0 || count > 5 {
			return nil, cfg, rules, nil, constUtil.ErrAdminConfiguration
		}
	}
	for index, claimed := range state.Claimed {
		if index < 1 || index > 7 || !claimed || index > len(state.Claimed) {
			return nil, cfg, rules, nil, constUtil.ErrAdminConfiguration
		}
	}
	if len(state.ClaimDays) != len(state.Claimed) || state.FinalClaimed && len(state.Claimed) != 7 {
		return nil, cfg, rules, nil, constUtil.ErrAdminConfiguration
	}
	return row, cfg, rules, state, nil
}

// 旧快照和回执保持原字节；仅对尚未发放的藏宝箱奖励纠正资源种类。
func reTreasureCoinRewards(rules *reOperationsWelfareRules) error {
	for i, tier := range rules.Tiers {
		items, err := p1RewardResources(tier.Reward)
		if err != nil || len(items) != 1 || (items[0].GetId() != "coin" && items[0].GetId() != "diamond") || items[0].GetNum() < 1 {
			return constUtil.ErrAdminConfiguration
		}
		rules.Tiers[i].Reward = "coin," + strconv.FormatInt(int64(items[0].GetNum()), 10)
	}
	return nil
}

func saveReOperationsWelfare(tx *gorm.DB, row *db.P4WelfareState, cfg db.P4WelfareRule, state *reOperationsWelfareProgress) error {
	row.Snapshot = adminJSON(cfg)
	row.Payload = adminJSON(state)
	return tx.Save(row).Error
}

func reFortuneCatInfo(rules reOperationsWelfareRules, state *reOperationsWelfareProgress, now time.Time) *pb.ActivityFortuneCatData {
	used := state.CatUses[p1Day(now)]
	cost := int32(0)
	status := int32(1)
	if used < 5 {
		cost = int32(rules.Tiers[used].Cost)
		status = 0
	}
	info := &pb.ActivityFortuneCatData{TodayLeft: proto.Int32(5 - used), TotalLeft: proto.Int32(5 - used), ThisTimeStatus: proto.Int32(status), TotalStatus: proto.Int32(0), CoinPool: proto.Int32(cost), LastGetTime: proto.Int64(state.LastGet), NextGetTime: proto.Int64(0), CurrentTime: proto.Int64(now.UnixMilli())}
	// Original fields 1..8 retain their schema. Tag 100 is a scalar quote extension.
	if used < 5 {
		resources, err := p1RewardResources(rules.Tiers[used].Reward)
		if err == nil && len(resources) == 1 && resources[0].GetId() == "coin" && resources[0].GetNum() > 0 {
			info.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 100, protowire.VarintType), uint64(resources[0].GetNum())))
		}
	}
	return info
}

// reFortuneCatWelfare keeps native cat requests and same-stamp receipts, with daily counts keyed by calendar date.
// 钻石消耗和金币奖励共用角色事务；已成功的旧请求继续返回原回执。
func (s *GameService) reFortuneCatWelfare(ctx context.Context, owner int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, bool, error) {
	query := db.GetIns().WithContext(ctx)
	key, err := reOperationsWelfareRuleKey(query, constUtil.ReWelfareCatKind, now)
	if err != nil {
		return nil, true, err
	}
	claim := false
	if _, ok := request.(*pb.CSFortuneCatReward); ok {
		claim = true
	}
	if key == "" {
		var receipts int64
		if claim {
			err = query.Model(&db.P4Receipt{}).Where("player_id=? AND domain=? AND timestamp=?", owner, constUtil.ReWelfareCatKind, stamp).Count(&receipts).Error
		}
		if err != nil {
			return nil, true, err
		}
		if receipts == 0 {
			return nil, false, nil
		}
	}
	var response proto.Message
	err = updateProfileWithTx(ctx, owner, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if claim {
			prior, e := p4ReadReceipt(tx, owner, constUtil.ReWelfareCatKind, stamp, request)
			if e != nil {
				return e
			}
			if prior != nil {
				response = prior
				return nil
			}
		}
		row, cfg, rules, state, e := s.loadReOperationsWelfareQuote(tx, owner, key, constUtil.ReWelfareCatKind, now, !claim)
		if e != nil {
			return e
		}
		if claim {
			b := request.(*pb.CSFortuneCatReward)
			used := state.CatUses[p1Day(now)]
			if b.GetType() != 0 || used >= 5 || now.UnixMilli() >= cfg.EndAt {
				return constUtil.ErrP4WelfareNotOpen
			}
			tier := rules.Tiers[used]
			if p.GetDiamond() < tier.Cost {
				return p3Error()
			}
			p.Diamond = proto.Int64(p.GetDiamond() - tier.Cost)
			gain, e := s.GrantFixedReward(p, g, tier.Reward)
			if e != nil {
				return e
			}
			state.CatUses[p1Day(now)]++
			state.LastGet = now.UnixMilli()
			response = &pb.SCFortuneCatReward{Get: gain, Info: reFortuneCatInfo(rules, state, now)}
			if e = p4SaveReceipt(tx, owner, constUtil.ReWelfareCatKind, stamp, request, response); e != nil {
				return e
			}
		} else {
			response = &pb.SCFortuneCatInfo{Info: reFortuneCatInfo(rules, state, now)}
		}
		return saveReOperationsWelfare(tx, row, cfg, state)
	})
	return []proto.Message{response}, true, err
}

func reContinuedSpent(tx *gorm.DB, owner int64, cfg db.P4WelfareRule, rules reOperationsWelfareRules, now time.Time) (int64, error) {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).UnixMilli()
	query, err := localPlayActivityFactScope(tx, cfg.ID)
	if err != nil {
		return 0, err
	}
	var facts []db.EventFact
	if err = query.Where("player_id=? AND kind=? AND day=? AND occurred_at>=? AND occurred_at<? AND occurred_at<=?", owner, constUtil.P1EventDiamondSpent, p1Day(now), max(cfg.StartAt, dayStart), min(cfg.EndAt, local.AddDate(0, 0, 1).UnixMilli()), now.UnixMilli()).Find(&facts).Error; err != nil {
		return 0, err
	}
	var spent int64
	for _, fact := range facts {
		if fact.Amount <= 0 {
			return 0, p3Error()
		}
		spent += min(fact.Amount, int64(math.MaxInt32)-spent)
		if spent == math.MaxInt32 {
			break
		}
	}
	return spent, nil
}
func reContinuedItems(rules reOperationsWelfareRules, state *reOperationsWelfareProgress, spent int64, now time.Time) ([]*pb.ActivityItem, int32, error) {
	count := len(state.Claimed)
	items := []*pb.ActivityItem{}
	for i, tier := range rules.Tiers {
		status := int32(0)
		if state.Claimed[i+1] {
			status = 2
		} else if i == count && spent >= rules.DailySpend && !state.ClaimDays[p1Day(now)] {
			status = 1
		}
		rewards, err := p1RewardResources(tier.Reward)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, &pb.ActivityItem{Index: proto.Int32(int32(i + 1)), Value: proto.String(strconv.FormatInt(rules.DailySpend, 10)), Param: proto.String(strconv.FormatInt(spent, 10)), RewardList: rewards, Status: proto.Int32(status), Left: proto.Int32(int32(min(spent, rules.DailySpend))), Total: proto.Int32(int32(rules.DailySpend)), PickOneOrGetAll: proto.Int32(1), Message: proto.String(fmt.Sprintf("第%d个消费日", i+1))})
	}
	status := int32(0)
	if count == 7 {
		status = 1
	}
	if state.FinalClaimed {
		status = 2
	}
	return items, status, nil
}

// reContinuedWelfare accepts the original continued-recharge protocol with real diamond spending eligibility.
// A day advances only on claiming its next earned tier; earlier unclaimed days are never backfilled.
func (s *GameService) reContinuedWelfare(ctx context.Context, owner int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var key string
	claim := false
	switch b := request.(type) {
	case *pb.CSContinuedRechargeInfo:
		key = b.GetActivityId()
	case *pb.CSContinuedRechargeReward:
		key = b.GetActivityId()
		claim = true
	default:
		return nil, p3Error()
	}
	if !strings.HasPrefix(key, constUtil.ReWelfareContinuedPrefix) {
		return nil, p3Error()
	}
	var response proto.Message
	err := updateProfileWithTx(ctx, owner, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if claim {
			prior, e := p4ReadReceipt(tx, owner, key, stamp, request)
			if e != nil {
				return e
			}
			if prior != nil {
				response = prior
				return nil
			}
		}
		row, cfg, rules, state, err := s.loadReOperationsWelfare(tx, owner, key, constUtil.ReWelfareContinuedKind, now)
		if err != nil {
			return err
		}
		spent, err := reContinuedSpent(tx, owner, cfg, rules, now)
		if err != nil {
			return err
		}
		if now.UnixMilli() >= cfg.EndAt {
			spent = 0
		}
		items, status, err := reContinuedItems(rules, state, spent, now)
		if err != nil {
			return err
		}
		gain := &pb.ResultObject{}
		if claim {
			b := request.(*pb.CSContinuedRechargeReward)
			reward := ""
			if b.GetType() == 1 {
				if b.GetIndex() != -1 || status != 1 {
					return p3Error()
				}
				reward = rules.FinalReward
				state.FinalClaimed = true
			} else if b.GetType() == 0 {
				index := b.GetIndex()
				if index < 1 || index > 7 || now.UnixMilli() >= cfg.EndAt || items[index-1].GetStatus() != 1 {
					return p3Error()
				}
				reward = rules.Tiers[index-1].Reward
				state.Claimed[int(index)] = true
				state.ClaimDays[p1Day(now)] = true
			} else {
				return p3Error()
			}
			expected, e := p1RewardResources(reward)
			if e != nil {
				return e
			}
			if !p1SameResources(b.GetGetResources(), expected) {
				return p3Error()
			}
			gain, err = s.GrantFixedReward(p, g, reward)
			if err != nil {
				return err
			}
			items, status, err = reContinuedItems(rules, state, spent, now)
			if err != nil {
				return err
			}
			response = &pb.SCContinuedRechargeReward{ActivityId: proto.String(key), ItemList: items, Get: gain, Status: proto.Int32(status)}
			if err = p4SaveReceipt(tx, owner, key, stamp, request, response); err != nil {
				return err
			}
		} else {
			big, e := p1RewardResources(rules.FinalReward)
			if e != nil {
				return e
			}
			response = &pb.SCContinuedRechargeInfo{ActivityId: proto.String(key), ItemList: items, StartTime: proto.Int64(cfg.StartAt), EndTime: proto.Int64(cfg.EndAt), RewardEndTime: proto.Int64(cfg.ClaimEndAt), Param: proto.String(strconv.FormatInt(rules.DailySpend, 10)), Information: proto.String("每周完成七个消费日；每个实际消费达标的登录日可领取一档"), Level: proto.Int32(7), CurrentIndex: proto.Int32(int32(min(7, len(state.Claimed)+1))), Status: proto.Int32(status), BigReward: big}
		}
		return saveReOperationsWelfare(tx, row, cfg, state)
	})
	return []proto.Message{response}, err
}

func (s *GameService) reOperationsWelfareTabs(tx *gorm.DB, now time.Time) ([]*pb.ActivityTab, error) {
	out := []*pb.ActivityTab{}
	for _, spec := range []struct{ kind, prefix, name string }{{constUtil.ReWelfareCatKind, constUtil.ReWelfareCatPrefix, "招财猫"}, {constUtil.ReWelfareContinuedKind, constUtil.ReWelfareContinuedPrefix, "连续消费"}} {
		key, err := reOperationsWelfareRuleKey(tx, spec.kind, now)
		if err != nil {
			return nil, err
		}
		if key == "" {
			continue
		}
		var cfg db.P4WelfareRule
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, "id=? AND kind=? AND enabled=?", key, spec.kind, true).Error; err != nil {
			return nil, err
		}
		if len(cfg.Frozen) > 0 && spec.kind != constUtil.ReWelfareCatKind {
			if json.Unmarshal(cfg.Frozen, &cfg) != nil {
				return nil, constUtil.ErrAdminConfiguration
			}
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.ClaimEndAt {
			continue
		}
		if err = s.validateReOperationsWelfare(cfg); err != nil {
			return nil, err
		}
		activityID := key
		if spec.kind == constUtil.ReWelfareCatKind {
			activityID = constUtil.ReWelfareCatPrefix + "current"
			// The original cat Info has no payout field. Carry the displayed frozen
			// five-tier quote in its no-ID page's tab identity without changing PB fields.
			var rules reOperationsWelfareRules
			if json.Unmarshal(cfg.Rules, &rules) != nil {
				return nil, constUtil.ErrAdminConfiguration
			}

			for _, tier := range rules.Tiers {
				resources, err := p1RewardResources(tier.Reward)
				if err != nil {
					return nil, err
				}
				activityID += fmt.Sprintf("~%d,%d", tier.Cost, resources[0].GetNum())
			}
		}
		out = append(out, &pb.ActivityTab{ActivityId: proto.String(activityID), Name: proto.String(spec.name), Order: proto.Int32(int32(35 + len(out))), Status: proto.Int32(0)})
	}
	return out, nil
}

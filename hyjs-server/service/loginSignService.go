package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
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

type p4SignDay struct {
	Rewards string   `json:"rewards"`
	Options []string `json:"options,omitempty"`
}
type p4Meal struct {
	StartMinute int   `json:"startMinute"`
	EndMinute   int   `json:"endMinute"`
	Amount      int32 `json:"amount"`
}
type p4CatPrize struct {
	Coin   int32 `json:"coin"`
	Weight int64 `json:"weight"`
}
type p4WelfareRules struct {
	RebateBps       int64        `json:"rebateBps,omitempty"`
	RebateCap       int64        `json:"rebateCap,omitempty"`
	Days            []p4SignDay  `json:"days,omitempty"`
	Meals           []p4Meal     `json:"meals,omitempty"`
	DayLimit        int32        `json:"dayLimit,omitempty"`
	TotalLimit      int32        `json:"totalLimit,omitempty"`
	CooldownSeconds int64        `json:"cooldownSeconds,omitempty"`
	ExploreReward   string       `json:"exploreReward,omitempty"`
	FinalBonus      string       `json:"finalBonus,omitempty"`
	CatPrizes       []p4CatPrize `json:"catPrizes,omitempty"`
}
type p4WelfareProgress struct {
	Claimed      map[int]bool    `json:"claimed,omitempty"`
	LastClaimDay string          `json:"lastClaimDay,omitempty"`
	SignCount    int             `json:"signCount,omitempty"`
	MealClaims   map[string]bool `json:"mealClaims,omitempty"`
	CatDay       string          `json:"catDay,omitempty"`
	CatToday     int32           `json:"catToday,omitempty"`
	CatTotal     int32           `json:"catTotal,omitempty"`
	CoinPool     int32           `json:"coinPool,omitempty"`
	NextGet      int64           `json:"nextGet,omitempty"`
	LastGet      int64           `json:"lastGet,omitempty"`
	CatFinal     bool            `json:"catFinal,omitempty"`
}

func (s *GameService) seedP4Welfare(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	dayRewards := []string{"coin,1000", "item_016,1", "item_005,10", "diamond,20", "item_002,1", "item_013,1", "item_048,1", "hero_exp_002,1", "item_021,1"}
	login := p4WelfareRules{}
	for _, content := range dayRewards {
		login.Days = append(login.Days, p4SignDay{Rewards: content})
	}
	login.Days = append(login.Days, p4SignDay{Options: []string{"item_021,2", "item_048,1", "hero_exp_006,1"}})
	limited := p4WelfareRules{}
	for _, content := range dayRewards[:7] {
		limited.Days = append(limited.Days, p4SignDay{Rewards: content})
	}
	specs := []struct {
		kind   string
		rules  p4WelfareRules
		source string
	}{
		{constUtil.P4WelfareRebate, p4WelfareRules{RebateBps: 1000, RebateCap: 100}, "本地兼容服消费返利：本月窗口内按北京时间当日实际钻石扣费返回10%，向下取整、上限100钻，每日一次；领取之后发生的消费不补算、不结转；返利本身不计消费，原累计消费活动由P3保留。"},
		{constUtil.P4WelfareLogin, login, "本地兼容服独立十日登录奖历：成功角色登录的不同北京时间日期计数，按序补领；前9档全领、第10档选一，活动结束7天内可补领已经达成档"},
		{constUtil.P4WelfareLimit, limited, "本地兼容服限时签到：本月窗口内每日一次，按7档依次领取，无补签；独立于普通月签到和豪华签到"},
		{constUtil.P4WelfareMeal, p4WelfareRules{Meals: []p4Meal{{720, 840, 50}, {1080, 1200, 50}}}, "本地兼容服月光餐厅：北京时间12:00-14:00/18:00-20:00，每窗口50体力，窗口外不补领；与制作料理和普通签到独立"},
		{constUtil.P4WelfareCat, p4WelfareRules{DayLimit: 3, TotalLimit: 15, CooldownSeconds: 7200, ExploreReward: "item_005,1", FinalBonus: "item_021,1", CatPrizes: []p4CatPrize{{3000, 80}, {10000, 20}}}, "本地兼容服藏宝箱：每日3次、每实例总15次、2小时冷却；探索给经验道具并积累金币池，3000/10000池增量权重80/20；15次后一次领取金币池及1神装精华"},
	}
	for _, spec := range specs {
		raw, err := json.Marshal(spec.rules)
		if err != nil {
			return err
		}
		cfg := db.P4WelfareRule{ID: "p4:" + spec.kind + ":" + start.Format("200601"), Kind: spec.kind, Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.AddDate(0, 0, 7).UnixMilli(), Rules: raw, Source: spec.source}
		if spec.kind == constUtil.P4WelfareLimit || spec.kind == constUtil.P4WelfareMeal || spec.kind == constUtil.P4WelfareRebate {
			cfg.ClaimEndAt = cfg.EndAt
		}
		var exists int64
		if err = tx.Model(&db.P4WelfareRule{}).Where("id=?", cfg.ID).Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		if _, err = s.validateP4Welfare(cfg); err != nil {
			return err
		}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) validateP4Welfare(cfg db.P4WelfareRule) (p4WelfareRules, error) {
	var rules p4WelfareRules
	if cfg.ID == "" || len(cfg.ID) > 64 || cfg.StartAt <= 0 || cfg.StartAt >= cfg.EndAt || cfg.ClaimEndAt < cfg.EndAt || json.Unmarshal(cfg.Rules, &rules) != nil {
		return rules, errors.New("福利实例规则无效")
	}
	switch cfg.Kind {
	case constUtil.P4WelfareRebate:
		if rules.RebateBps < 1 || rules.RebateBps > 10000 || rules.RebateCap < 1 || rules.RebateCap > math.MaxInt32 {
			return rules, errors.New("返利比例或上限无效")
		}
	case constUtil.P4WelfareLogin, constUtil.P4WelfareLimit:
		if len(rules.Days) < 1 || len(rules.Days) > 31 {
			return rules, errors.New("福利奖历缺失")
		}
		for _, day := range rules.Days {
			if (day.Rewards == "") == (len(day.Options) == 0) || len(day.Options) > 3 {
				return rules, errors.New("福利全领或选一规则无效")
			}
			if cfg.Kind == constUtil.P4WelfareLimit {
				resources, err := p1RewardResources(day.Rewards)
				if err != nil || len(resources) != 1 || len(day.Options) > 0 {
					return rules, errors.New("限时签到协议只接受单项奖励")
				}
			}
			rewards := day.Options
			if day.Rewards != "" {
				rewards = []string{day.Rewards}
			}
			seen := map[string]bool{}
			for _, content := range rewards {
				ok, err := s.openServerRewardSupported(content)
				if err != nil || !ok {
					return rules, errors.New("福利奖励资源无效")
				}
				if len(day.Options) > 0 {
					res, err := p1RewardResources(content)
					if err != nil || len(res) != 1 || seen[res[0].GetId()] {
						return rules, errors.New("福利选一选项无效")
					}
					seen[res[0].GetId()] = true
				}
			}
		}
	case constUtil.P4WelfareMeal:
		if len(rules.Meals) != 2 {
			return rules, errors.New("原餐厅界面要求两个时间窗")
		}
		previousEnd := -1
		for _, window := range rules.Meals {
			if window.StartMinute < 0 || window.EndMinute > 1440 || window.StartMinute >= window.EndMinute || window.StartMinute < previousEnd || window.Amount <= 0 {
				return rules, errors.New("餐厅时间窗或体力无效")
			}
			previousEnd = window.EndMinute
		}
	case constUtil.P4WelfareCat:
		if rules.DayLimit != 3 || rules.TotalLimit < 1 || rules.TotalLimit > 1000 || rules.CooldownSeconds < 0 || rules.CooldownSeconds > 604800 || len(rules.CatPrizes) == 0 {
			return rules, errors.New("藏宝箱次数或冷却无效")
		}
		var totalWeight int64
		for _, prize := range rules.CatPrizes {
			if prize.Coin <= 0 || int64(prize.Coin)*int64(rules.TotalLimit) > math.MaxInt32 || prize.Weight <= 0 || totalWeight > math.MaxInt64-prize.Weight {
				return rules, errors.New("藏宝箱奖池无效")
			}
			totalWeight += prize.Weight
		}
		for _, content := range []string{rules.ExploreReward, rules.FinalBonus} {
			ok, err := s.openServerRewardSupported(content)
			if err != nil || !ok {
				return rules, errors.New("藏宝箱奖励无效")
			}
		}
	default:
		return rules, errors.New("福利类型未支持")
	}
	return rules, nil
}

// Character schedules own normal P4 states and receipts. Only legacy demo
// cycles have retired snapshots in the separate local-play ledger.
func p4WelfareRetired(tx *gorm.DB, ruleID string) (*localPlayRetiredWelfare, error) {
	if strings.Contains(ruleID, "_character_") {
		return nil, nil
	}
	return localPlayWelfareRetired(tx, ruleID)
}

func (s *GameService) loadP4Welfare(tx *gorm.DB, id int64, kind string, now time.Time) (*db.P4WelfareState, db.P4WelfareRule, p4WelfareRules, *p4WelfareProgress, error) {
	var empty db.P4WelfareRule
	var zero p4WelfareRules
	if err := s.seedP4Welfare(tx, now); err != nil {
		return nil, empty, zero, nil, err
	}
	scopedKey, err := localPlayWelfareRuleID(tx, kind)
	if err != nil {
		return nil, empty, zero, nil, err
	}
	// A new-player programme owns an ordinary native consumer per role.
	// Keep old demo-cycle precedence and global pending-order behavior when
	// no authenticated role-specific window is currently applicable.
	if scopedKey == "" {
		scopedKey, err = AdminCurrentCharacterWelfareRuleKey(tx, kind, now.UnixMilli())
		if err != nil {
			return nil, empty, zero, nil, err
		}
	}
	var configs []db.P4WelfareRule
	order := "start_at DESC, id DESC"
	if kind == constUtil.P4WelfareLogin || kind == constUtil.P4WelfareCat {
		// 原协议没有实例ID，先补领宽限期内已经达成的旧实例，再展示当前实例。
		order = "start_at ASC, id ASC"
	}
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND enabled = ?", kind, true)
	var pendingKeys []string
	if scopedKey != "" {
		pendingKeys = []string{scopedKey}
		if !strings.Contains(scopedKey, "_character_") && (kind == constUtil.P4WelfareLogin || kind == constUtil.P4WelfareCat) {
			pendingKeys, err = localPlayWelfarePendingKeys(tx, id, kind, scopedKey)
			if err != nil {
				return nil, empty, zero, nil, err
			}
		}
		query = query.Where("id IN ?", pendingKeys)
	}
	if err := query.Order(order).Find(&configs).Error; err != nil {
		return nil, empty, zero, nil, err
	}
	if len(pendingKeys) > 1 {
		rank := map[string]int{}
		for i, key := range pendingKeys {
			rank[key] = i
		}
		sort.SliceStable(configs, func(i, j int) bool { return rank[configs[i].ID] < rank[configs[j].ID] })
	}
	for _, live := range configs {
		retired, e := p4WelfareRetired(tx, live.ID)
		if e != nil {
			return nil, empty, zero, nil, e
		}
		allowed, err := localPlayActivityVisible(tx, live.ID)
		if err != nil {
			return nil, empty, zero, nil, err
		}
		if !allowed && retired == nil {
			continue
		}
		cfg := live
		if len(live.Frozen) > 0 {
			if err := json.Unmarshal(live.Frozen, &cfg); err != nil {
				return nil, empty, zero, nil, err
			}
			cfg.Enabled = live.Enabled
		}
		rules, err := s.validateP4Welfare(cfg)
		if err != nil {
			return nil, empty, zero, nil, err
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.ClaimEndAt {
			continue
		}
		if len(live.Frozen) == 0 {
			frozen, err := json.Marshal(cfg)
			if err != nil {
				return nil, empty, zero, nil, err
			}
			if err = tx.Model(&live).Update("frozen", frozen).Error; err != nil {
				return nil, empty, zero, nil, err
			}
		}
		row := &db.P4WelfareState{PlayerID: id, RuleID: cfg.ID}
		state := &p4WelfareProgress{Claimed: map[int]bool{}, MealClaims: map[string]bool{}}
		err = tx.Where("player_id = ? AND rule_id = ?", id, cfg.ID).First(row).Error
		if err == nil {
			if json.Unmarshal(row.Snapshot, &cfg) != nil || json.Unmarshal(row.Payload, state) != nil {
				return nil, empty, zero, nil, errors.New("福利冻结存档损坏")
			}
			rules, err = s.validateP4Welfare(cfg)
			if err != nil {
				return nil, empty, zero, nil, err
			}
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, empty, zero, nil, err
		}
		if state.Claimed == nil {
			state.Claimed = map[int]bool{}
		}
		if state.MealClaims == nil {
			state.MealClaims = map[string]bool{}
		}
		if state.SignCount < 0 || state.SignCount > len(rules.Days) || state.CatTotal < 0 || state.CatTotal > rules.TotalLimit || state.CoinPool < 0 {
			return nil, empty, zero, nil, errors.New("福利进度无效")
		}
		if retired != nil {
			visible := *retired
			visible.LoginDays = localPlayWelfareRetiredDays(retired, now)
			if !localPlayWelfarePending(&visible, rules, state) || (kind == constUtil.P4WelfareCat && state.LastGet > now.UnixMilli()) {
				continue
			}
			// Only already earned frozen rights enter this old no-ID page.
			// Pending selection never accepts another old exploration or login.
			return row, cfg, rules, state, nil
		}
		if now.UnixMilli() >= cfg.EndAt {
			switch kind {
			case constUtil.P4WelfareLogin:
				var days int64
				facts, e := localPlayActivityFactScope(tx, cfg.ID)
				if e != nil {
					return nil, empty, zero, nil, e
				}
				if err = facts.Model(&db.EventFact{}).Where("player_id=? AND kind=? AND occurred_at>=? AND occurred_at<? AND occurred_at<=?", id, constUtil.P1EventLoginDay, cfg.StartAt, cfg.EndAt, now.UnixMilli()).Distinct("day").Count(&days).Error; err != nil {
					return nil, empty, zero, nil, err
				}
				pending := false
				for i := range rules.Days {
					pending = pending || (int64(i) < days && !state.Claimed[i])
				}
				if !pending {
					continue
				}
			case constUtil.P4WelfareCat:
				if state.CatTotal < rules.TotalLimit || state.CatFinal {
					continue
				}
			}
		}
		return row, cfg, rules, state, nil
	}
	return nil, empty, zero, nil, constUtil.ErrP4WelfareNotOpen
}

func saveP4Welfare(tx *gorm.DB, row *db.P4WelfareState, cfg db.P4WelfareRule, state *p4WelfareProgress) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	row.Snapshot = raw
	row.Payload, err = json.Marshal(state)
	if err != nil {
		return err
	}
	return tx.Save(row).Error
}
func p4Kind(request proto.Message) string {
	switch request.(type) {
	case *pb.CSLoginRewardInfo, *pb.CSGetLoginReward:
		return constUtil.P4WelfareLogin
	case *pb.CSRewardLimitSignList, *pb.CSReceiveLimitMonthSign:
		return constUtil.P4WelfareLimit
	case *pb.CSStaminaRefill, *pb.CSStaminaGet:
		return constUtil.P4WelfareMeal
	default:
		return constUtil.P4WelfareCat
	}
}
func p4Meals(cfg db.P4WelfareRule, rules p4WelfareRules, state *p4WelfareProgress, now time.Time) ([]*pb.ActivityStaminaData, int) {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	day := p1Day(now)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	active := -1
	var data []*pb.ActivityStaminaData
	for i, w := range rules.Meals {
		start := midnight.Add(time.Duration(w.StartMinute) * time.Minute).UnixMilli()
		end := midnight.Add(time.Duration(w.EndMinute) * time.Minute).UnixMilli()
		status := int32(0)
		if state.MealClaims[fmt.Sprintf("%s:%d", day, i)] {
			status = 2
		} else if now.UnixMilli() >= start && now.UnixMilli() < end && now.UnixMilli() < cfg.EndAt {
			status = 1
			active = i
		}
		data = append(data, &pb.ActivityStaminaData{StartTime: proto.Int64(start), EndTime: proto.Int64(end), Status: proto.Int32(status)})
	}
	return data, active
}
func p4CatData(rules p4WelfareRules, state *p4WelfareProgress, now time.Time) *pb.ActivityFortuneCatData {
	today := state.CatToday
	if state.CatDay != p1Day(now) {
		today = 0
	}
	total := int32(0)
	if state.CatFinal {
		total = 2
	} else if state.CatTotal >= rules.TotalLimit {
		total = 1
	}
	status := int32(0)
	if today >= rules.DayLimit {
		status = 2
	} else if now.UnixMilli() < state.NextGet {
		status = 1
	}
	return &pb.ActivityFortuneCatData{TodayLeft: proto.Int32(max(0, rules.DayLimit-today)), TotalLeft: proto.Int32(max(0, rules.TotalLimit-state.CatTotal)), LastGetTime: proto.Int64(state.LastGet), ThisTimeStatus: proto.Int32(status), TotalStatus: proto.Int32(total), CoinPool: proto.Int32(state.CoinPool), NextGetTime: proto.Int64(state.NextGet), CurrentTime: proto.Int64(now.UnixMilli())}
}

// p4Welfare 所有领奖与回执在角色行锁事务中；只读查询不制造登录日或补签事实。
func (s *GameService) p4Welfare(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		kind := p4Kind(request)
		operation := p4ResponseName(request) != ""
		if operation {
			prior, err := p4ReadReceipt(tx, id, "welfare:"+kind, stamp, request)
			if err != nil {
				return err
			}
			if prior != nil {
				response = prior
				return nil
			}
		}
		row, cfg, rules, state, err := s.loadP4Welfare(tx, id, kind, now)
		// 原客户端进主城会主动查登录奖；未配置时明确隐藏，不制造奖励或报登录错误。
		if errors.Is(err, constUtil.ErrP4WelfareNotOpen) {
			if _, query := request.(*pb.CSLoginRewardInfo); query {
				response = &pb.SCLoginRewardInfo{IsShow: proto.Int32(0), HasReward: proto.Int32(0)}
				return nil
			}
		}
		if err != nil {
			return err
		}
		gain := &pb.ResultObject{}
		switch b := request.(type) {
		case *pb.CSLoginRewardInfo, *pb.CSGetLoginReward:
			var days int64
			retired, e := p4WelfareRetired(tx, cfg.ID)
			if e != nil {
				return e
			}
			if retired != nil {
				days = localPlayWelfareRetiredDays(retired, now)
			} else {
				facts, e := localPlayActivityFactScope(tx, cfg.ID)
				if e != nil {
					return e
				}
				if err = facts.Model(&db.EventFact{}).Where("player_id = ? AND kind = ? AND occurred_at >= ? AND occurred_at < ? AND occurred_at<=?", id, constUtil.P1EventLoginDay, cfg.StartAt, cfg.EndAt, now.UnixMilli()).Distinct("day").Count(&days).Error; err != nil {
					return err
				}
			}
			next := -1
			for i := range rules.Days {
				if !state.Claimed[i] {
					next = i
					break
				}
			}
			if req, ok := request.(*pb.CSGetLoginReward); ok {
				if next < 0 || int64(next) >= days {
					return errors.New("独立登录奖励条件未达成")
				}
				day := rules.Days[next]
				content := day.Rewards
				if len(day.Options) > 0 {
					content = ""
					for _, option := range day.Options {
						resources, e := p1RewardResources(option)
						if e != nil {
							return e
						}
						if resources[0].GetId() == req.GetItemId() {
							content = option
						}
					}
					if content == "" {
						return errors.New("登录奖励选项无效")
					}
				} else if req.GetItemId() != "" {
					return errors.New("全领奖励无需选择资源")
				}
				gain, err = s.GrantFixedReward(p, g, content)
				if err != nil {
					return err
				}
				state.Claimed[next] = true
				response = &pb.SCGetLoginReward{Get: gain}
			} else {
				result := &pb.SCLoginRewardInfo{IsShow: proto.Int32(1), HasReward: proto.Int32(0)}
				if next < 0 {
					result.IsShow = proto.Int32(0)
				}
				for i, day := range rules.Days {
					status := int32(0)
					if state.Claimed[i] {
						status = 2
					} else if i == next && int64(i) < days {
						status = 1
						result.HasReward = proto.Int32(1)
					}
					contents := day.Options
					all := int32(2)
					if day.Rewards != "" {
						contents = []string{day.Rewards}
						all = 1
					}
					var resources []*pb.BaseResource
					for _, content := range contents {
						r, e := p1RewardResources(content)
						if e != nil {
							return e
						}
						resources = append(resources, r...)
					}
					result.Items = append(result.Items, &pb.LoginRewardItem{Day: proto.Int32(int32(i + 1)), Rewards: resources, State: proto.Int32(status), PickOneOrGetAll: proto.Int32(all)})
				}
				response = result
			}
		case *pb.CSRewardLimitSignList, *pb.CSReceiveLimitMonthSign:
			if _, ok := request.(*pb.CSReceiveLimitMonthSign); ok {
				if now.UnixMilli() >= cfg.EndAt || state.SignCount >= len(rules.Days) {
					return errors.New("限时签到已结束或已全部领取")
				}
				if state.LastClaimDay >= p1Day(now) {
					response = &pb.SCReceiveLimitMonthSign{Get: gain}
					break
				}
				gain, err = s.GrantFixedReward(p, g, rules.Days[state.SignCount].Rewards)
				if err != nil {
					return err
				}
				state.SignCount++
				state.LastClaimDay = p1Day(now)
				response = &pb.SCReceiveLimitMonthSign{Get: gain}
			} else {
				data := &pb.LimitDaySignData{SignNum: proto.Int32(int32(state.SignCount)), AllNum: proto.Int32(int32(len(rules.Days)))}
				for i, day := range rules.Days {
					status := int32(0)
					if i < state.SignCount {
						status = 2
					} else if i == state.SignCount && state.LastClaimDay < p1Day(now) && now.UnixMilli() < cfg.EndAt {
						status = 1
					}
					data.List = append(data.List, &pb.LimitSignRewardItem{GetState: proto.Int32(status), RewardId: proto.String(day.Rewards), ItemKeyId: proto.Int32(int32(i + 1))})
				}
				response = &pb.SCRewardLimitSignList{LimitDaySignDatas: data}
			}
		case *pb.CSStaminaRefill, *pb.CSStaminaGet:
			data, index := p4Meals(cfg, rules, state, now)
			if _, ok := request.(*pb.CSStaminaGet); ok {
				if index < 0 {
					return errors.New("月光餐厅当前没有可领窗口")
				}
				gain, err = s.GrantFixedReward(p, g, fmt.Sprintf("stamina,%d", rules.Meals[index].Amount))
				if err != nil {
					return err
				}
				state.MealClaims[fmt.Sprintf("%s:%d", p1Day(now), index)] = true
				data, _ = p4Meals(cfg, rules, state, now)
				response = &pb.SCStaminaGet{Get: gain, DataList: data}
			} else {
				response = &pb.SCStaminaRefill{DataList: data}
			}
		case *pb.CSFortuneCatInfo:
			response = &pb.SCFortuneCatInfo{Info: p4CatData(rules, state, now)}
		case *pb.CSFortuneCatReward:
			data := p4CatData(rules, state, now)
			if b.GetType() != data.GetTotalStatus() || state.CatFinal {
				return errors.New("藏宝箱请求阶段无效")
			}
			if b.GetType() == 1 {
				gain, err = s.GrantFixedReward(p, g, "coin,"+strconv.FormatInt(int64(state.CoinPool), 10)+";"+rules.FinalBonus)
				if err != nil {
					return err
				}
				state.CatFinal = true
				state.NextGet = 0
			} else if b.GetType() == 0 {
				if now.UnixMilli() >= cfg.EndAt || data.GetTodayLeft() <= 0 || now.UnixMilli() < state.NextGet {
					return errors.New("藏宝箱窗口/次数/冷却未满足")
				}
				var weight int64
				for _, prize := range rules.CatPrizes {
					weight += prize.Weight
				}
				draw, e := rand.Int(rand.Reader, big.NewInt(weight))
				if e != nil {
					return e
				}
				pick := draw.Int64()
				var coin int32
				for _, prize := range rules.CatPrizes {
					pick -= prize.Weight
					if pick < 0 {
						coin = prize.Coin
						break
					}
				}
				if coin <= 0 || int64(state.CoinPool)+int64(coin) > math.MaxInt32 {
					return errors.New("藏宝箱金币池溢出")
				}
				gain, err = s.GrantFixedReward(p, g, rules.ExploreReward)
				if err != nil {
					return err
				}
				if state.CatDay != p1Day(now) {
					state.CatDay = p1Day(now)
					state.CatToday = 0
				}
				state.CatToday++
				state.CatTotal++
				state.CoinPool += coin
				state.LastGet = now.UnixMilli()
				state.NextGet = now.Add(time.Duration(rules.CooldownSeconds) * time.Second).UnixMilli()
				if state.CatTotal >= rules.TotalLimit {
					state.NextGet = 0
				}
				if err = s.RecordP1Event(tx, id, fmt.Sprintf("cat:%s:%d", cfg.ID, state.CatTotal), "fortune_explore", 1, now); err != nil {
					return err
				}
			} else {
				return errors.New("藏宝箱类型无效")
			}
			response = &pb.SCFortuneCatReward{Get: gain, Info: p4CatData(rules, state, now)}
		default:
			return errors.New("福利请求类型无效")
		}
		if err = saveP4Welfare(tx, row, cfg, state); err != nil {
			return err
		}
		if operation {
			return p4SaveReceipt(tx, id, "welfare:"+kind, stamp, request, response)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"strings"
	"time"
)

// WelfareAchievementRedPoints 只读真实达标与领取状态，不因菜单查询冻结规则或生成奖品。
// 每个负责的原类型均返回零值，以便最后一项领取后清除旧红点。
func (s *GameService) WelfareAchievementRedPoints(ctx context.Context, id int64) (*pb.SCRedPoint, error) {
	var out *pb.SCRedPoint
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.Profile
		if err := tx.First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		out, err = s.welfareAchievementRedPoints(ctx, tx, id, player, game)
		return err
	})
	return out, err
}

func (s *GameService) welfareAchievementRedPoints(ctx context.Context, tx *gorm.DB, id int64, player *pb.Player, game *model.GameState) (*pb.SCRedPoint, error) {
	values := map[string]bool{}
	var err error
	// 日卡和会员提示复用实际支付及领取资格，查询本身不创建领取行。
	if s.redPointHasTable(tx, &db.PaymentOrder{}) && s.redPointHasTable(tx, &db.MembershipClaim{}) {
		now := LocalPlayNow(ctx)
		for _, kind := range []string{"month", "permanent"} {
			status, _, e := s.cardStatus(tx, player, kind, now)
			if e != nil {
				return nil, e
			}
			values["monthCardReward"] = values["monthCardReward"] || status == "canClaim"
		}
		daily, _, e := s.vipBoon(tx, player, now)
		if e != nil {
			return nil, e
		}
		for _, item := range daily {
			values["vipBoon"] = values["vipBoon"] || item.GetStatus() == 1
		}
	}
	ordinary, e := s.monthSignReadyPoint(tx, id, player, LocalPlayNow(ctx))
	if e != nil {
		return nil, e
	}
	luxury, e := s.luxurySignStatus(tx, id, LocalPlayNow(ctx))
	if e != nil {
		return nil, e
	}
	values["dailySignOrdinary"] = ordinary
	values["dailySignLuxury"] = luxury == 1
	values["dailySign"] = ordinary || luxury == 1
	if s.redPointHasTable(tx, &db.P5Achievement{}) {
		ready, e := s.achievementReady(tx, id, player, game)
		if e != nil {
			return nil, e
		}
		values["achievement"] = ready
	}
	if s.redPointHasTable(tx, &db.PeriodInstance{}) {
		if err = s.periodReadyPoints(tx, id, values); err != nil {
			return nil, err
		}
	}
	if s.redPointHasTable(tx, &db.P4WelfareRule{}) {
		if err = s.welfareReadyPoints(tx, id, values); err != nil {
			return nil, err
		}
	}
	if s.redPointHasTable(tx, &db.PaymentActivityRule{}) {
		var rules []db.PaymentActivityRule
		now := LocalPlayNow(ctx)
		if err = tx.Where("enabled=? AND start_at<=? AND reward_end_at>?", true, now.UnixMilli(), now.UnixMilli()).Find(&rules).Error; err != nil {
			return nil, err
		}
		for _, cfg := range rules {
			allowed, e := localPlayActivityVisible(tx, cfg.ID)
			if e != nil {
				return nil, e
			}
			if !allowed {
				continue
			}
			var state db.PaymentActivityState
			e = tx.First(&state, "player_id=? AND activity_id=?", id, cfg.ID).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return nil, e
			}
			if state.Claimed {
				continue
			}
			if state.Snapshot != "" {
				if json.Unmarshal([]byte(state.Snapshot), &cfg) != nil {
					continue
				}
			}
			value, e := p3ActivityMetric(tx, id, &cfg, now)
			if e != nil {
				return nil, e
			}
			if value >= cfg.Threshold {
				kind := map[string]string{"first": "firstRechargeGift", "single": "rechargeSingle", "total": "rechargeTotal", "consume": "costTotal", "continued": "continuedRecharge"}[cfg.Kind]
				if kind != "" {
					values[kind] = true
				}
			}
		}
	}
	if s.redPointHasTable(tx, &db.ActivityRules{}) {
		if err = fundReadyPoints(tx, id, player, values); err != nil {
			return nil, err
		}
	}
	out := &pb.SCRedPoint{}
	keys := []string{"dailySign", "dailySignOrdinary", "dailySignLuxury", "monthCardReward", "vipBoon", "achievement", "totalLogin", "rechargeSingle", "firstRechargeGift", "rechargeTotal", "fightRedNameTimes", "killRedNameTimes", "robTreasureTimes", "pvpWinTimes", "mainCopyWinTimes", "aincradStars", "costTotal", "continuedRecharge", "warriorRescruitTimes", "warriorGetSpecial", "kaiGongActivity", "limitSignOpen", "moonlightRestaurant", "openServerFund", "openServerFundChild", "allPeopleBoon"}
	for _, key := range keys {
		value := int32(0)
		if values[key] {
			value = 1
		}
		out.RedPointList = append(out.RedPointList, &pb.RedPoint{Type: proto.String(key), Value: proto.Int32(value)})
	}
	// 原菜单单独监听父类型；子项可领取时才显示活动入口红点。
	activityReady := int32(0)
	for _, point := range out.RedPointList {
		if point.GetType() != "achievement" && point.GetValue() > 0 {
			activityReady = 1
			break
		}
	}
	out.RedPointList = append(out.RedPointList, &pb.RedPoint{Type: proto.String("wonderfulActivity"), Value: proto.Int32(activityReady)})
	return out, nil
}

// monthSignReadyPoint reads the same calendar and scoped state as the claim path.
// Polling never creates a sign row, freezes a reward or changes a receipt.
func (s *GameService) monthSignReadyPoint(tx *gorm.DB, id int64, player *pb.Player, now time.Time) (bool, error) {
	if !s.redPointHasTable(tx, &db.MonthSignRule{}) || !s.redPointHasTable(tx, &db.MonthSignState{}) || !s.redPointHasTable(tx, &db.MonthSignCalendar{}) {
		return false, nil
	}
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	month, day := local.Format("2006-01"), local.Format("2006-01-02")
	calendarDays := time.Date(local.Year(), local.Month()+1, 0, 0, 0, 0, 0, local.Location()).Day()
	rules, configured, err := monthSignCalendarRules(tx, month)
	if err != nil {
		return false, err
	}
	if !configured {
		rules, err = localPlayMonthSignRules(tx)
		if err != nil {
			return false, err
		}
		if rules == nil {
			if err = tx.Order("day").Find(&rules).Error; err != nil {
				return false, err
			}
		}
	}
	if !validMonthSignRows(rules) {
		return false, nil
	}
	for _, rule := range rules[:calendarDays] {
		if !rule.Enabled {
			return false, nil
		}
		supported, err := s.openServerRewardSupported(rule.Reward)
		if err != nil {
			return false, err
		}
		if !supported {
			return false, nil
		}
	}
	state, err := localPlayMonthSignState(tx, id, month)
	if err != nil {
		return false, err
	}
	if state == nil {
		state = &db.MonthSignState{PlayerID: id, Month: month}
		err = tx.Where("player_id=? AND month=?", id, month).First(state).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	}
	return monthSignStateReadyPoint(state, player.GetVip(), day, int32(calendarDays)), nil
}

func monthSignStateReadyPoint(state *db.MonthSignState, vip int32, day string, calendarDays int32) bool {
	if state.Days < 0 || state.Days > calendarDays || state.LastDay > day || (state.Days > 0 && (state.LastDay == "" || state.LastReward == "" || state.LastDoubleVIP < 0)) {
		return false
	}
	if state.LastDay == day {
		return !state.Doubled && state.LastDoubleVIP > 0 && vip >= state.LastDoubleVIP
	}
	return state.Days < calendarDays
}

func (s *GameService) achievementReady(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState) (bool, error) {
	var rows []db.P5Achievement
	if err := tx.Order("id").Find(&rows).Error; err != nil {
		return false, err
	}
	for _, cfg := range rows {
		var state db.P5AchievementClaim
		e := tx.First(&state, "player_id=? AND id=?", id, cfg.ID).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return false, e
		}
		if state.ClaimedAt > 0 || cfg.Disabled && errors.Is(e, gorm.ErrRecordNotFound) {
			continue
		}
		if state.Snapshot != "" {
			if json.Unmarshal([]byte(state.Snapshot), &cfg) != nil {
				return false, constUtil.ErrAdminConfiguration
			}
		}
		value, e := s.p5AchievementValue(tx, p, g, &cfg)
		if e != nil {
			return false, e
		}
		acquired, e := p5ReGrowthAlreadyAcquired(tx, id, g, &cfg)
		if e != nil {
			return false, e
		}
		if !acquired && max(value, state.Progress) >= cfg.Target {
			return true, nil
		}
	}
	return false, nil
}

func (s *GameService) periodReadyPoints(tx *gorm.DB, id int64, values map[string]bool) error {
	now := LocalPlayNow(tx.Statement.Context).UnixMilli()
	var rows []db.PeriodInstance
	if err := tx.Where("enabled=?", true).Find(&rows).Error; err != nil {
		return err
	}
	for _, live := range rows {
		allowed, e := localPlayActivityVisible(tx, live.ID)
		if e != nil {
			return e
		}
		if !allowed {
			continue
		}
		cfg, e := s.freezeP1Period(tx, live, false)
		if e != nil {
			continue
		}
		if now < cfg.StartAt || now >= cfg.RewardEndAt || isP15SeasonalChild(cfg.ID) {
			continue
		}
		progress, frozen, e := s.p1Progress(tx, cfg, id)
		if e != nil {
			return e
		}
		cfg = frozen
		value := progress.Value
		// 原查询会补齐新发布主线活动的已提交事实；红点复用同一SUM而不保存。
		if cfg.Kind == constUtil.P1EventCopyWin && cfg.Mode == "cumulative" && now < cfg.EndAt {
			facts, e := localPlayActivityFactScope(tx, cfg.ID)
			if e != nil {
				return e
			}
			var total int64
			if e = facts.Model(&db.EventFact{}).Select("COALESCE(SUM(amount),0)").Where("player_id=? AND kind=? AND amount>0 AND occurred_at>=? AND occurred_at<? AND occurred_at<=?", id, cfg.Kind, cfg.StartAt, cfg.EndAt, now).Scan(&total).Error; e != nil {
				return e
			}
			value = max(value, total)
		}
		if cfg.Mode == "daily" && progress.LastDay != p1Day(time.UnixMilli(now)) && now < cfg.EndAt {
			value = 0
		}
		sources := p1PeriodSources(cfg)
		for index, tier := range p1PeriodMilestones(cfg) {
			var delivery db.RewardDelivery
			e = tx.First(&delivery, "player_id=? AND source_key=?", id, sources[index]).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			ready := delivery.ID > 0 && delivery.ClaimedAt == 0 && (delivery.ExpiresAt == 0 || delivery.ExpiresAt > now)
			if errors.Is(e, gorm.ErrRecordNotFound) && value >= tier.Goal {
				ready = true
			}
			if ready {
				values[strings.SplitN(cfg.ID, "_", 2)[0]] = true
			}
		}
	}
	return nil
}

func (s *GameService) welfareReadyPoints(tx *gorm.DB, id int64, values map[string]bool) error {
	now := LocalPlayNow(tx.Statement.Context)
	var rows []db.P4WelfareRule
	if err := tx.Where("enabled=?", true).Order("start_at DESC,id DESC").Find(&rows).Error; err != nil {
		return err
	}
	for _, live := range rows {
		allowed, e := localPlayActivityVisible(tx, live.ID)
		if e != nil {
			return e
		}
		if !allowed {
			continue
		}
		cfg := live
		if len(cfg.Frozen) > 0 && json.Unmarshal(cfg.Frozen, &cfg) != nil {
			return constUtil.ErrAdminConfiguration
		}
		var saved db.P4WelfareState
		e = tx.First(&saved, "player_id=? AND rule_id=?", id, cfg.ID).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if len(saved.Snapshot) > 0 && json.Unmarshal(saved.Snapshot, &cfg) != nil {
			return constUtil.ErrAdminConfiguration
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.ClaimEndAt {
			continue
		}
		if cfg.Kind == constUtil.ReWelfareContinuedKind {
			key, e := reOperationsWelfareRuleKey(tx, cfg.Kind, now)
			if e != nil {
				return e
			}
			if key != cfg.ID {
				continue
			}
			var rules reOperationsWelfareRules
			state := &reOperationsWelfareProgress{Claimed: map[int]bool{}, ClaimDays: map[string]bool{}}
			if json.Unmarshal(cfg.Rules, &rules) != nil || len(saved.Payload) > 0 && json.Unmarshal(saved.Payload, state) != nil {
				return constUtil.ErrAdminConfiguration
			}
			spent, e := reContinuedSpent(tx, id, cfg, rules, now)
			if e != nil {
				return e
			}
			items, status, e := reContinuedItems(rules, state, spent, now)
			if e != nil {
				return e
			}
			if status == 1 {
				values["continuedRecharge"] = true
			}
			for _, item := range items {
				if item.GetStatus() == 1 {
					values["continuedRecharge"] = true
				}
			}
			continue
		}
		if cfg.Kind != constUtil.P4WelfareLogin && cfg.Kind != constUtil.P4WelfareLimit && cfg.Kind != constUtil.P4WelfareMeal {
			continue
		}
		rules, e := s.validateP4Welfare(cfg)
		if e != nil {
			return e
		}
		state := &p4WelfareProgress{Claimed: map[int]bool{}, MealClaims: map[string]bool{}}
		if len(saved.Payload) > 0 && json.Unmarshal(saved.Payload, state) != nil {
			return constUtil.ErrAdminConfiguration
		}
		switch cfg.Kind {
		case constUtil.P4WelfareLogin:
			facts, e := localPlayActivityFactScope(tx, cfg.ID)
			if e != nil {
				return e
			}
			var days int64
			if e = facts.Model(&db.EventFact{}).Where("player_id=? AND kind=? AND occurred_at>=? AND occurred_at<? AND occurred_at<=?", id, constUtil.P1EventLoginDay, cfg.StartAt, cfg.EndAt, now.UnixMilli()).Distinct("day").Count(&days).Error; e != nil {
				return e
			}
			for i := range rules.Days {
				if !state.Claimed[i] {
					if int64(i) < days {
						values["totalLogin"] = true
					}
					break
				}
			}
		case constUtil.P4WelfareLimit:
			if state.SignCount < len(rules.Days) && state.LastClaimDay < p1Day(now) && now.UnixMilli() < cfg.EndAt {
				values["limitSignOpen"] = true
			}
		case constUtil.P4WelfareMeal:
			_, index := p4Meals(cfg, rules, state, now)
			if index >= 0 {
				values["moonlightRestaurant"] = true
			}
		}
	}
	return nil
}

// fundReadyPoints 复用原基金购买人数与角色等级口径，购物动作本身不亮点。
func fundReadyPoints(tx *gorm.DB, id int64, p *pb.Player, values map[string]bool) error {
	var cfg db.ActivityRules
	e := tx.First(&cfg, "id=? AND enabled=?", "open_server_fund", true).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	now := LocalPlayNow(tx.Statement.Context).UnixMilli()
	if now < cfg.StartAt || now >= cfg.RewardEndAt {
		return nil
	}
	allowed, e := localPlayActivityVisible(tx, "openServerFund_local_20261011")
	if e != nil {
		return e
	}
	if !allowed {
		return nil
	}
	var state db.ActivityState
	e = tx.First(&state, "activity_id=? AND player_id=?", cfg.ID, id).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	claims := map[string]bool{}
	if len(state.Claims) > 0 && json.Unmarshal(state.Claims, &claims) != nil {
		return constUtil.ErrAdminConfiguration
	}
	var count int64
	if e = tx.Model(&db.ActivityState{}).Where("activity_id=? AND bought=?", cfg.ID, true).Count(&count).Error; e != nil {
		return e
	}
	var rewards []db.ActivityReward
	if e = tx.Where("activity_id=?", cfg.ID).Find(&rewards).Error; e != nil {
		return e
	}
	for _, row := range rewards {
		if claims[row.RewardType+":"+strconv.Itoa(int(row.Threshold))] {
			continue
		}
		if row.RewardType == "openServerFund" && state.Bought && p.GetLevel() >= row.Threshold {
			values["openServerFundChild"] = true
		}
		if row.RewardType == "everyoneBenefit" && count >= int64(activityRewardDisplayIndex(row)) {
			values["allPeopleBoon"] = true
		}
	}
	values["openServerFund"] = values["openServerFundChild"] || values["allPeopleBoon"]
	return nil
}

// afterWelfareAchievementRequest 在原事务提交后刷新资格，涵盖进度、领取和目录初次打开。
func (s *GameService) afterWelfareAchievementRequest(ctx context.Context, id int64, request proto.Message, responses []proto.Message) {
	if !welfareAchievementRequestRefresh(request, responses) {
		return
	}
	if points, err := s.WelfareAchievementRedPoints(ctx, id); err == nil {
		s.publishP10Private(ctx, id, points)
	}
}

func welfareAchievementRequestRefresh(request proto.Message, responses []proto.Message) bool {
	if request == nil {
		return false
	}
	refresh := false
	switch string(request.ProtoReflect().Descriptor().Name()) {
	case "CSRewardMonthSignList", "CSReceiveMonthSign", "CSReceiveLuxurySign", "CSIsCardClaimed", "CSClaimCard", "CSGetVipBoonInfo", "CSGetVipDailyBoonReward", "CSGetVipWeekBoonReward", "CSAchievementList", "CSGetAchievementReward", "CSActivityList", "CSActivityInfo", "CSActivityReward", "CSLoginRewardInfo", "CSGetLoginReward", "CSRewardLimitSignList", "CSReceiveLimitMonthSign", "CSStaminaRefill", "CSStaminaGet", "CSContinuedRechargeInfo", "CSContinuedRechargeReward", "CSOpenServerFundInfo", "CSBuyOpenServerFund", "CSOpenServerFundReward":
		refresh = true
	}
	if !refresh {
		for _, response := range responses {
			if response != nil && inventoryResourceResponse(response.ProtoReflect(), 0) {
				refresh = true
				break
			}
		}
	}
	return refresh
}

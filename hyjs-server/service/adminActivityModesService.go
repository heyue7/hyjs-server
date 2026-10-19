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
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

type adminActivityDefinition struct {
	mode, label, domain, kind, prefix, prefab string
	products, milestones, prizes              bool
	maxMilestones                             int64
}

// adminActivityDefinitions 绑定已实现原消费者与实际原UI prefab；未恢复页面不虚构为可运营模式。
func adminActivityDefinitions() []adminActivityDefinition {
	return []adminActivityDefinition{
		{"shop-window", "限时商店", "commerce", "shop-window", "shop_schedule_", "Shop", false, false, false, 0},
		{"exchange", "资源兑换", "commerce", "exchange", "limitExchange_p4_admin_", "ActivityInnerPage/OnSail", true, false, false, 0},
		{"discount-shop", "折扣礼包", "commerce", "discount", "discountSale_p4_admin_", "ActivityInnerPage/OnSail", true, false, false, 0},
		{"half-price", "半价抢购", "commerce", "half", "halfDiscountSale_p4_admin_", "ActivityInnerPage/OnSail", true, false, false, 0},
		{"limited-shop", "限时特卖", "commerce", "sale", "limitSale_p4_admin_", "ActivityInnerPage/OnSail", true, false, false, 0},
		{"mystic-shop", "神秘小屋", "commerce", "mystic", "mysticDiscount_p4_admin_", "ActivityInnerPage/MysticDiscount", true, false, false, 0},
		{"login-rewards", "登录奖历", "welfare", "login", "p4:login:admin_", "ActivityInnerPage/login award", false, true, false, 31},
		{"limited-sign", "限时签到", "welfare", "limit", "p4:limit:admin_", "ActivityInnerPage/every day sign", false, true, false, 31},
		{"stamina-meal", "月光餐厅", "welfare", "meal", "p4:meal:admin_", "ActivityInnerPage/asuna kitchen", false, false, false, 0},
		{"diamond-rebate", "消费返利", "welfare", "rebate", "p4:rebate:admin_", "ActivityInnerPage/CostReturn", false, false, false, 0},
		{"treasure-box", "藏宝箱", "welfare", "cat", "p4:cat:admin_", "ActivityInnerPage/treasurecast", false, false, false, 0},
		{"turntable", "幸运转盘", "pool", "turntable", "admin_turntable_", "ActivityInnerPage/BigTurnTable", false, false, true, 0},
		{"recharge-total", "累计充值", "period", "payment_amount", "rechargeTotal_p4_admin_", "ActivityInnerPage/recharge surprise", false, true, false, 1},
		{"diamond-spent", "累计消费", "period", constUtil.P1EventDiamondSpent, "costTotal_p4_admin_", "ActivityInnerPage/recharge surprise", false, true, false, 1},
		{"recruit-count", "累计招募", "period", constUtil.P1EventRecruit, "warriorRescruitTimes_p4_admin_", "ActivityInnerPage/recharge surprise", false, true, false, 1},
		{"copy-win", "主线胜利", "period", constUtil.P1EventCopyWin, "mainCopyWinTimes_p4_admin_", "ActivityInnerPage/BattleRace", false, true, false, 1},
		{"arena-wins", "竞技胜场", "period", constUtil.P4ArenaWinKind, "pvpWinTimes_p4_arena_admin_", "ActivityInnerPage/BattleRace", false, true, false, 1},
		{"plunder-count", "夺宝次数", "period", constUtil.P4PlunderActivityKind, "robTreasureTimes_p4_admin_", "ActivityInnerPage/BattleRace", false, true, false, 1},
		{"hero-collection", "英雄收藏", "period", constUtil.P4KindCollect, "warriorGetSpecial_p4_admin_", "ActivityInnerPage/collection warrior", false, true, false, 1},
		{"trial-stars", "试炼星数", "period", constUtil.P7RushStarEvent, "aincradStars_p4_admin_", "ActivityInnerPage/BattleRace", false, true, false, 1},
		{"power-race", "战力竞赛", "period", constUtil.P4KindPower, "kaiFuJingSai_p4_admin_", "ActivityInnerPage/open server race", false, true, false, 1},
		{"king-recruit", "王者招募", "pool", "king-recruit", "admin_king_recruit_", "ShopInnerPage/KingRecruit", false, false, true, 0},
		{"login-total", "累计登录", "period", constUtil.P1EventLoginDay, "totalLogin_p1_admin_", "ActivityInnerPage/login award", false, true, false, 1},
	}
}
func adminActivityDefinitionFor(mode string) (adminActivityDefinition, bool) {
	for _, d := range adminActivityDefinitions() {
		if d.mode == mode {
			return d, true
		}
	}
	return adminActivityDefinition{}, false
}

// InitializeAdminActivityModes 仅迁移展示关联表，不创建未知活动、奖品或运营投放。
func InitializeAdminActivityModes(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminActivityPresentation{})
}

// AdminActivityModes 提供按原消费者裁剪的字段、资格枚举及空奖品模板。
func (s *GameService) AdminActivityModes() []model.AdminActivityMode {
	result := []model.AdminActivityMode{}
	vip := []model.AdminConditionOption{}
	for _, row := range s.Tables["VipPerks"] {
		v, err := strconv.ParseInt(row["vip"], 10, 64)
		if err == nil {
			vip = append(vip, model.AdminConditionOption{Label: fmt.Sprintf("VIP%d", v), Value: v})
		}
	}
	for _, d := range adminActivityDefinitions() {
		form := model.AdminActivityForm{Mode: d.mode, TemplateID: d.mode + "-original", Name: d.label, Source: "运营配置", Products: []model.AdminActivityProduct{}, Milestones: []model.AdminActivityMilestone{}, Prizes: []model.AdminActivityPrize{}}
		mode := model.AdminActivityMode{Mode: d.mode, Label: d.label, Templates: []model.AdminActivityTemplate{{ID: form.TemplateID, Name: d.label + "原版页面", Prefab: d.prefab, Preview: ""}}, Conditions: []model.AdminActivityCondition{}, SupportsProducts: d.products, SupportsMilestones: d.milestones, SupportsPrizes: d.prizes, MaxMilestones: d.maxMilestones, Notes: "只修改今后未冻结的新一期；空奖品草稿保持关闭。", DefaultForm: form}
		if s.AdminCatalogueData != nil {
			if preview := s.AdminCatalogueData.ActivityPreviews[d.mode]; preview != "" {
				mode.Templates[0].Preview = preview
			}
		}
		if d.products {
			mode.Conditions = []model.AdminActivityCondition{{Key: "minLevel", Label: "角色等级", Type: "number"}, {Key: "minVip", Label: "最低VIP", Type: "select", Options: vip}}
		}
		if d.domain == "period" {
			mode.Notes += " 当前原消费者每实例支持1档目标；累计充值目标单位为分。"
		}
		if d.mode == "king-recruit" {
			mode.FixedValues = map[string]int64{"dayLimit": 50, "luckyGoal": 500, "luckyPerDraw": 10, "rotationHours": 48}
			mode.Notes = "沿用原王者招募页面、阵营轮换和价格；日限、保底与幸运进度跨排期保留。未排期前沿用现有常驻池，首次排期后由本服日历控制。"
		}
		if d.mode == "power-race" {
			mode.Notes = "记录真实阵容战力峰值；目标奖励与截止排名奖励分开配置，同一排名范围使用同档奖励。"
		}
		if d.mode == "trial-stars" {
			mode.Notes = "读取真实试炼结算的累计星数峰值；每期目标与奖励独立。"
		}
		if d.mode == "shop-window" {
			mode.Notes = "从商店管理关闭不限时后排期；商品与价格取该商店的已保存目录。"
			mode.DefaultForm.Options.ShopKey = "ordinary"
		}
		if d.mode == "turntable" {
			for i := int64(0); i < 9; i++ {
				mode.DefaultForm.Prizes = append(mode.DefaultForm.Prizes, model.AdminActivityPrize{ID: i})
			}
			mode.FixedValues = map[string]int64{"singleDiamond": 50, "tenDiamond": 488, "coinPerDraw": 10000}
			mode.DefaultForm.Options.ScorePerDraw = 1
			mode.Notes += " 奖品库存、钻石大奖池和记录按角色独立计算，不发排名奖。"
		}
		if d.mode == "stamina-meal" {
			mode.DefaultForm.Options.Meals = []model.AdminMealWindow{{StartMinute: 720, EndMinute: 840}, {StartMinute: 1080, EndMinute: 1200}}
		}
		if d.mode == "treasure-box" {
			mode.DefaultForm.Options.DayLimit = 3
		}
		result = append(result, mode)
	}
	return result
}

func adminTypedResources(content string) []model.AdminResourceAmount {
	out := []model.AdminResourceAmount{}
	if content == "" {
		return out
	}
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			continue
		}
		amount, err := strconv.ParseInt(pair[1], 10, 64)
		if err == nil {
			out = append(out, model.AdminResourceAmount{ResourceID: pair[0], Amount: amount})
		}
	}
	return out
}
func adminTypedResourceOne(content string) model.AdminResourceAmount {
	values := adminTypedResources(content)
	if len(values) == 1 {
		return values[0]
	}
	return model.AdminResourceAmount{}
}
func (s *GameService) adminTypedResourceSpec(values []model.AdminResourceAmount, optional bool) (string, error) {
	if len(values) == 0 {
		if optional {
			return "", nil
		}
		return "", constUtil.ErrAdminInput
	}
	if len(values) > 100 {
		return "", constUtil.ErrAdminInput
	}
	seen := map[string]bool{}
	parts := []string{}
	for _, r := range values {
		if r.ResourceID == "" || strings.ContainsAny(r.ResourceID, ",; \r\n") || r.Amount < 1 || r.Amount > math.MaxInt32 || seen[r.ResourceID] {
			return "", constUtil.ErrAdminInput
		}
		seen[r.ResourceID] = true
		parts = append(parts, fmt.Sprintf("%s,%d", r.ResourceID, r.Amount))
	}
	content := strings.Join(parts, ";")
	ok, err := s.openServerRewardSupported(content)
	if err != nil || !ok {
		return "", constUtil.ErrAdminInput
	}
	return content, nil
}

func adminActivityRaw(tx *gorm.DB, id string) (model.AdminActivityDTO, bool, error) {
	// A lock-bearing builder is reusable only after Session enables per-query statement cloning.
	// Keep the requested row lock while preventing the first table/WHERE from leaking into pool reward reads.
	tx = tx.Session(&gorm.Session{})
	domain, key, ok := strings.Cut(id, ":")
	if !ok {
		return model.AdminActivityDTO{}, false, constUtil.ErrAdminInput
	}
	switch domain {
	case "commerce":
		var row db.P4CommerceRule
		if err := tx.First(&row, "id=?", key).Error; err != nil {
			return model.AdminActivityDTO{}, false, err
		}
		return adminActivityProjection(domain, row, nil), len(row.Frozen) > 0, nil
	case "welfare":
		var row db.P4WelfareRule
		if err := tx.First(&row, "id=?", key).Error; err != nil {
			return model.AdminActivityDTO{}, false, err
		}
		return adminActivityProjection(domain, row, nil), len(row.Frozen) > 0, nil
	case "period":
		var row db.PeriodInstance
		if err := tx.First(&row, "id=?", key).Error; err != nil {
			return model.AdminActivityDTO{}, false, err
		}
		return adminActivityProjection(domain, row, nil), len(row.Frozen) > 0, nil
	case "pool":
		var row db.GameplayPool
		if err := tx.First(&row, "id=?", key).Error; err != nil {
			return model.AdminActivityDTO{}, false, err
		}
		var rewards []db.GameplayPoolReward
		if err := tx.Where("pool_id=?", key).Order("id").Find(&rewards).Error; err != nil {
			return model.AdminActivityDTO{}, false, err
		}
		var count int64
		if tx.Migrator().HasTable(&db.TurntableState{}) {
			// The already locked pool identifies this archive; aggregate history reads need no FOR UPDATE.
			if err := tx.Session(&gorm.Session{NewDB: true}).Model(&db.TurntableState{}).Where("pool_id=?", key).Count(&count).Error; err != nil {
				return model.AdminActivityDTO{}, false, err
			}
		}
		kingFrozen, err := adminKingRecruitActivityFrozen(tx, key)
		if err != nil {
			return model.AdminActivityDTO{}, false, err
		}
		return adminActivityProjection(domain, row, rewards), count > 0 || kingFrozen, nil
	}
	return model.AdminActivityDTO{}, false, constUtil.ErrAdminInput
}

// adminActivityReadableName 解析历史生成名称，保留运营已输入的可读标题。
func adminActivityReadableName(name, id, kind, mode string) string {
	readable := false
	for _, r := range name {
		if unicode.Is(unicode.Han, r) {
			readable = true
			break
		}
	}
	if readable && !strings.ContainsRune(name, '\ufffd') {
		return name
	}
	if name != "" && !strings.ContainsAny(name, "_:·") && !strings.ContainsRune(name, '\ufffd') {
		return name
	}
	if d, ok := adminActivityDefinitionFor(mode); ok {
		for _, named := range []struct{ key, label string }{
			{"Festival", "节日资源兑换"}, {"awakeelement", "觉醒元素兑换"}, {"awakematerial", "觉醒材料兑换"},
			{"crystals", "晶石兑换"}, {"fireworks", "烟花兑换"}, {"gloryfrag", "荣耀碎片兑换"},
			{"ingredients", "料理食材兑换"}, {"petfood", "精灵口粮兑换"}, {"red_os", "红色角色碎片兑换"},
			{"specialty", "专精材料兑换"}, {"worldseed", "世界之种兑换"}, {"growth_materials", "成长材料兑换"},
			{"_gem_", "宝石兑换"}, {"_gold_", "金币兑换"},
		} {
			if mode == "exchange" && strings.Contains(id, named.key) {
				return named.label
			}
		}
		return d.label
	}
	if label := map[string]string{"guild": "公会商店", "memory": "记忆圣坛", "ordinary": "普通商店", "point": "攻略点商店", "popularity-rank": "声望排名商店", "popularity": "声望商店", "score": "积分商店"}[kind]; label != "" {
		return label
	}
	return "待整理活动"
}

// adminActivityAvailability 只检查配置与后台适配，不改变开关、不创建奖品。
func (s *GameService) adminActivityAvailability(tx *gorm.DB, raw model.AdminActivityDTO, form model.AdminActivityForm, supported bool) model.AdminActivityAvailability {
	result := model.AdminActivityAvailability{Usable: true, Missing: []string{}}
	if !supported {
		result.Usable = false
		result.Reason = "此类型尚未适配活动编辑器，保留原功能与数据"
		result.Missing = []string{"缺少对应的活动编辑模式；本页仅供查看"}
		return result
	}
	if strings.Contains(raw.ID, "lp-") {
		result.Usable = false
		result.Reason = "旧钱包生成的个人活动期，已停止作为全服运营入口"
		result.Missing = []string{"请从活动管理新建全服活动；旧期仅保留查询与历史记录"}
		return result
	}
	var problem error
	switch raw.Type {
	case "commerce":
		var cfg db.P4CommerceRule
		problem = adminDecodeActivityRule(raw.Config, &cfg)
		if problem == nil {
			_, problem = s.validateP4Commerce(cfg)
		}
	case "welfare":
		var cfg db.P4WelfareRule
		problem = adminDecodeActivityRule(raw.Config, &cfg)
		if problem == nil {
			_, problem = s.validateP4Welfare(cfg)
		}
	case "period":
		var cfg db.PeriodInstance
		problem = json.Unmarshal(raw.Config, &cfg)
		if problem == nil {
			problem = s.validateP1Period(cfg)
		}
	case "pool":
		var cfg struct {
			Pool    db.GameplayPool         `json:"pool"`
			Rewards []db.GameplayPoolReward `json:"rewards"`
		}
		problem = json.Unmarshal(raw.Config, &cfg)
		if problem == nil {
			if form.Mode == "king-recruit" {
				_, problem = s.adminKingRecruitActivityForm(tx, cfg.Pool, cfg.Rewards)
				break
			}
			options := form.Options
			top, advance := "", ""
			if options.TopPrize.ResourceID != "" {
				top = fmt.Sprintf("%s,%d", options.TopPrize.ResourceID, options.TopPrize.Amount)
			}
			if options.AdvancePrize.ResourceID != "" {
				advance = fmt.Sprintf("%s,%d", options.AdvancePrize.ResourceID, options.AdvancePrize.Amount)
			}
			rules := db.TurntableRules{PoolID: cfg.Pool.ID, ScorePerDraw: int32(options.ScorePerDraw), AdvancePlaces: int32(options.AdvancePlaces), TopPrize: top, AdvancePrize: advance}
			problem = s.validateTurntableConfiguration(&cfg.Pool, cfg.Rewards, &rules)
		}
	}
	if problem != nil {
		result.Usable = false
		result.Reason = "配置待补齐"
		result.Missing = []string{problem.Error()}
	}
	return result
}

func (s *GameService) adminActivityTypedProjection(tx *gorm.DB, raw model.AdminActivityDTO, frozen bool) (model.AdminActivityInstance, error) {
	form := model.AdminActivityForm{Name: raw.Name, Source: raw.Source, Products: []model.AdminActivityProduct{}, Milestones: []model.AdminActivityMilestone{}, Prizes: []model.AdminActivityPrize{}}
	kind := ""
	switch raw.Type {
	case "commerce":
		var cfg db.P4CommerceRule
		if adminDecodeActivityRule(raw.Config, &cfg) != nil {
			return model.AdminActivityInstance{}, constUtil.ErrAdminInput
		}
		kind = cfg.Kind
		form.StartAt = cfg.StartAt
		form.EndAt = cfg.EndAt
		form.ClaimEndAt = cfg.ClaimEndAt
		var rules p4CommerceRules
		_ = json.Unmarshal(cfg.Rules, &rules)
		form.Options.Discounts = rules.Discounts
		for _, p := range rules.Products {
			form.Products = append(form.Products, model.AdminActivityProduct{ID: p.ID, Name: p.Name, Index: int64(p.Index), Rewards: adminTypedResources(p.Rewards), Cost: adminTypedResourceOne(p.Cost), DailyLimit: int64(p.DailyLimit), TotalLimit: int64(p.TotalLimit), Stock: int64(p.Stock), MinLevel: int64(p.MinLevel), MinVip: int64(p.MinVIP), DiscountBps: int64(p.DiscountBps), Weight: p.Weight})
		}
	case "welfare":
		var cfg db.P4WelfareRule
		if adminDecodeActivityRule(raw.Config, &cfg) != nil {
			return model.AdminActivityInstance{}, constUtil.ErrAdminInput
		}
		kind = cfg.Kind
		form.StartAt = cfg.StartAt
		form.EndAt = cfg.EndAt
		form.ClaimEndAt = cfg.ClaimEndAt
		var rules p4WelfareRules
		_ = json.Unmarshal(cfg.Rules, &rules)
		form.Options.RebateBps = rules.RebateBps
		form.Options.RebateCap = rules.RebateCap
		form.Options.DayLimit = int64(rules.DayLimit)
		form.Options.TotalLimit = int64(rules.TotalLimit)
		form.Options.CooldownSeconds = rules.CooldownSeconds
		form.Options.ExploreRewards = adminTypedResources(rules.ExploreReward)
		form.Options.FinalRewards = adminTypedResources(rules.FinalBonus)
		for _, w := range rules.Meals {
			form.Options.Meals = append(form.Options.Meals, model.AdminMealWindow{StartMinute: w.StartMinute, EndMinute: w.EndMinute, Amount: int64(w.Amount)})
		}
		for _, v := range rules.CatPrizes {
			form.Options.CoinPrizes = append(form.Options.CoinPrizes, model.AdminCoinPrize{Coin: int64(v.Coin), Weight: v.Weight})
		}
		for i, v := range rules.Days {
			m := model.AdminActivityMilestone{Goal: int64(i + 1), Rewards: adminTypedResources(v.Rewards)}
			for _, option := range v.Options {
				m.Options = append(m.Options, adminTypedResourceOne(option))
			}
			form.Milestones = append(form.Milestones, m)
		}
	case "period":
		var cfg db.PeriodInstance
		if json.Unmarshal(raw.Config, &cfg) != nil {
			return model.AdminActivityInstance{}, constUtil.ErrAdminInput
		}
		kind = cfg.Kind
		if cfg.Mode != "cumulative" && cfg.Kind != constUtil.P4KindPower && cfg.Kind != constUtil.P7RushStarEvent || cfg.RankPlaces > 0 && cfg.Kind != constUtil.P4KindPower {
			kind = "unsupported"
		}
		form.StartAt = cfg.StartAt
		form.EndAt = cfg.EndAt
		form.ClaimEndAt = cfg.RewardEndAt
		form.Milestones = []model.AdminActivityMilestone{{Goal: cfg.Goal, Rewards: adminTypedResources(cfg.Rewards)}}
		if cfg.Kind == constUtil.P4KindPower {
			form.Options.RankPlaces = int64(cfg.RankPlaces)
			form.Options.RankRewards = adminTypedResources(cfg.RankRewards)
		}
	case "pool":
		var cfg struct {
			Pool    db.GameplayPool         `json:"pool"`
			Rewards []db.GameplayPoolReward `json:"rewards"`
		}
		if json.Unmarshal(raw.Config, &cfg) != nil {
			return model.AdminActivityInstance{}, constUtil.ErrAdminInput
		}
		if cfg.Pool.ID == "recruit_3" || strings.HasPrefix(cfg.Pool.ID, "admin_king_recruit_") {
			kingForm, _ := s.adminKingRecruitActivityForm(tx, cfg.Pool, cfg.Rewards)
			form = kingForm
			kind = "king-recruit"
			break
		}
		var wheel db.TurntableRules
		err := gorm.ErrRecordNotFound
		if tx.Migrator().HasTable(&db.TurntableRules{}) {
			err = tx.First(&wheel, "pool_id=?", cfg.Pool.ID).Error
		}
		if err == nil {
			kind = "turntable"
			form.Options.ScorePerDraw = int64(wheel.ScorePerDraw)
			form.Options.TopPrize = adminTypedResourceOne(wheel.TopPrize)
			form.Options.AdvancePrize = adminTypedResourceOne(wheel.AdvancePrize)
			form.Options.AdvancePlaces = int64(wheel.AdvancePlaces)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AdminActivityInstance{}, err
		}
		form.StartAt = cfg.Pool.StartAt
		form.EndAt = cfg.Pool.EndAt
		form.ClaimEndAt = cfg.Pool.RewardEndAt
		form.Options.InitialDiamondPool = cfg.Pool.InitialDiamondPool
		form.Options.JackpotPerMillion = int64(cfg.Pool.JackpotPerMillion)
		form.Options.PoolContributionBps = int64(cfg.Pool.PoolContributionBps)
		for _, p := range cfg.Rewards {
			form.Prizes = append(form.Prizes, model.AdminActivityPrize{ID: int64(p.ID), ResourceID: p.ResourceID, Amount: int64(p.Amount), Weight: p.Weight, Stock: int64(p.Stock), Guaranteed: p.Guaranteed})
		}
	}
	supported := false
	for _, d := range adminActivityDefinitions() {
		if d.domain == raw.Type && d.kind == kind {
			form.Mode = d.mode
			form.TemplateID = d.mode + "-original"
			supported = true
			break
		}
	}
	if !supported {
		form.Mode = "unsupported"
	}
	var meta db.AdminActivityPresentation
	err := tx.First(&meta, "activity_id=?", raw.ID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AdminActivityInstance{}, err
	}
	if err == nil {
		form.Name = meta.Name
		form.TemplateID = meta.TemplateID
	}
	form.Name = adminActivityReadableName(form.Name, raw.ID, kind, form.Mode)
	for i := range form.Products {
		product := &form.Products[i]
		if strings.Contains(product.Name, "name_") || product.Name == "" || product.Name == product.ID {
			product.Name = fmt.Sprintf("商品%d", i+1)
			if len(product.Rewards) > 0 && s.AdminCatalogueData != nil {
				for _, item := range s.AdminCatalogueData.Items {
					if item.ID == product.Rewards[0].ResourceID {
						product.Name = item.Name
						break
					}
				}
			}
		}
	}
	availability := s.adminActivityAvailability(tx, raw, form, supported)
	return model.AdminActivityInstance{ID: raw.ID, Name: form.Name, Mode: form.Mode, TemplateID: form.TemplateID, Enabled: raw.Enabled, Version: adminVersion(struct {
		Source   string
		Metadata db.AdminActivityPresentation
	}{raw.Version, meta}), Frozen: frozen, Editable: supported && !frozen && !strings.Contains(raw.ID, "lp-"), Form: form, Availability: availability}, nil
}

// AdminActivityInstances 返回业务字段及冻结标记，不暴露JSON规则编辑器。
func (s *GameService) AdminActivityInstances(ctx context.Context) ([]model.AdminActivityInstance, error) {
	rows, err := AdminActivities(ctx)
	if err != nil {
		return nil, err
	}
	tx := db.GetIns().WithContext(ctx)
	out := []model.AdminActivityInstance{}
	for _, row := range rows {
		if row.Type == "registry" || strings.Contains(row.ID, "_character_") {
			continue
		}
		var meta db.AdminActivityPresentation
		err := tx.First(&meta, "activity_id=?", row.ID).Error
		if err == nil && meta.Archived {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		raw, frozen, err := adminActivityRaw(tx, row.ID)
		if err != nil {
			return nil, err
		}
		dto, err := s.adminActivityTypedProjection(tx, raw, frozen)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// AdminSaveTypedActivity 将模式表单适配到原消费者规则，资格写入原商品字段，在原交易入口强制生效。
func (s *GameService) AdminSaveTypedActivity(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminTypedActivityRequest) (*model.AdminActivityInstance, error) {
	definition, ok := adminActivityDefinitionFor(req.AdminActivityForm.Mode)
	if !ok || req.AdminActivityForm.TemplateID != definition.mode+"-original" {
		return nil, constUtil.ErrAdminInput
	}
	if id == "" {
		id = definition.domain + ":" + definition.prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	}
	var saved model.AdminActivityInstance
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		domain, key, _ := strings.Cut(id, ":")
		if domain != definition.domain {
			return constUtil.ErrAdminInput
		}
		raw, frozen, lookup := adminActivityRaw(tx.Clauses(clause.Locking{Strength: "UPDATE"}), id)
		exists := lookup == nil
		if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		if exists {
			current, err := s.adminActivityTypedProjection(tx, raw, frozen)
			if err != nil {
				return err
			}
			if req.Version != current.Version || current.Mode != req.AdminActivityForm.Mode || !current.Editable {
				return constUtil.ErrAdminConfigConflict
			}
		} else if req.Version != "" {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Mode == "king-recruit" {
			form, err := s.adminKingRecruitCanonicalForm(tx, req.AdminActivityForm)
			if err != nil {
				return err
			}
			req.AdminActivityForm = form
		}
		if err := s.adminPersistTypedActivity(tx, definition, key, req.AdminActivityForm, req.Enabled, exists); err != nil {
			return err
		}
		if req.MakeCurrent {
			if !req.Enabled || definition.mode != "turntable" {
				return constUtil.ErrAdminInput
			}
			var king db.KingRecruitRules
			if err := tx.First(&king, 1).Error; err != nil {
				return err
			}
			if err := tx.Model(&king).Update("wheel_pool", key).Error; err != nil {
				return err
			}
		}
		meta := db.AdminActivityPresentation{ActivityID: id, Mode: req.AdminActivityForm.Mode, TemplateID: req.AdminActivityForm.TemplateID, Name: req.AdminActivityForm.Name, UpdatedAtMillis: time.Now().UnixMilli()}
		if err := tx.Save(&meta).Error; err != nil {
			return err
		}
		raw, frozen, err := adminActivityRaw(tx, id)
		if err != nil {
			return err
		}
		saved, err = s.adminActivityTypedProjection(tx, raw, frozen)
		if err != nil {
			return err
		}
		action := "activity.create"
		if exists {
			action = "activity.update"
		}
		return adminAudit(tx, p, action, id, "mode="+definition.mode+" version="+saved.Version)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *GameService) adminPersistTypedActivity(tx *gorm.DB, d adminActivityDefinition, key string, f model.AdminActivityForm, enabled, exists bool) error {
	if f.Mode == "king-recruit" {
		return s.adminPersistKingRecruitActivity(tx, key, f, enabled, exists)
	}
	if f.Mode == "shop-window" {
		return s.adminPersistShopSchedule(tx, key, f, enabled, exists)
	}
	if f.RepeatPolicy != "" && f.RepeatPolicy != "once" && f.RepeatPolicy != "repeat" {
		return constUtil.ErrAdminInput
	}
	if f.Options.DayLimit < 0 || f.Options.DayLimit > math.MaxInt32 || f.Options.TotalLimit < 0 || f.Options.TotalLimit > math.MaxInt32 || f.Options.ScorePerDraw < 0 || f.Options.ScorePerDraw > math.MaxInt32 || f.Options.AdvancePlaces < 0 || f.Options.AdvancePlaces > 1000 || f.Options.InitialDiamondPool < 0 || f.Options.InitialDiamondPool > math.MaxInt32 || f.Options.JackpotPerMillion < 0 || f.Options.JackpotPerMillion > 1000000 || f.Options.PoolContributionBps < 0 || f.Options.PoolContributionBps > 10000 {
		return constUtil.ErrAdminInput
	}
	if strings.TrimSpace(f.Name) == "" || len([]rune(f.Name)) > 80 || f.StartAt <= 0 || f.EndAt <= f.StartAt || f.ClaimEndAt < f.EndAt {
		return constUtil.ErrAdminInput
	}
	save := func(value any) error {
		if exists {
			return tx.Save(value).Error
		}
		return tx.Create(value).Error
	}
	switch d.domain {
	case "commerce":
		rules := p4CommerceRules{Products: []p4Product{}, Discounts: f.Options.Discounts}
		for i, p := range f.Products {
			rewards, err := s.adminTypedResourceSpec(p.Rewards, !enabled)
			if err != nil {
				return err
			}
			cost := ""
			if p.Cost.ResourceID != "" || p.Cost.Amount > 0 {
				cost = fmt.Sprintf("%s,%d", p.Cost.ResourceID, p.Cost.Amount)
				if _, err := s.p4ParseCost(cost); err != nil {
					return constUtil.ErrAdminInput
				}
			}
			if p.MinVip < 0 || p.MinVip > 12 || p.MinLevel < 1 || p.MinLevel > 1000 || p.DailyLimit < 1 || p.DailyLimit > 10000 || p.TotalLimit < 0 || p.TotalLimit > math.MaxInt32 || p.Stock < -1 || p.Stock > math.MaxInt32 || p.DiscountBps < 1 || p.DiscountBps > 10000 || p.Weight < 1 || p.Weight > 1000000000 {
				return constUtil.ErrAdminInput
			}
			productID := p.ID
			if productID == "" {
				productID = key + fmt.Sprintf("_%d", i+1)
			}
			index := p.Index
			if index < 1 {
				index = int64(i + 1)
			}
			rules.Products = append(rules.Products, p4Product{ID: productID, Name: p.Name, Index: int32(index), Rewards: rewards, Cost: cost, Priority: int32(1000 + i), DailyLimit: int32(p.DailyLimit), TotalLimit: int32(p.TotalLimit), Stock: int32(p.Stock), MinLevel: int32(p.MinLevel), MinVIP: int32(p.MinVip), DiscountBps: int32(p.DiscountBps), Weight: p.Weight})
		}
		cfg := db.P4CommerceRule{ID: key, Kind: d.kind, Enabled: enabled, StartAt: f.StartAt, EndAt: f.EndAt, ClaimEndAt: f.ClaimEndAt, Rules: adminJSON(rules), Source: f.Source}
		if enabled || len(rules.Products) > 0 {
			if _, err := s.validateP4Commerce(cfg); err != nil {
				return adminActivityValidationError(err)
			}
		}
		return save(&cfg)
	case "welfare":
		rules := p4WelfareRules{RebateBps: f.Options.RebateBps, RebateCap: f.Options.RebateCap, DayLimit: int32(f.Options.DayLimit), TotalLimit: int32(f.Options.TotalLimit), CooldownSeconds: f.Options.CooldownSeconds}
		for _, m := range f.Milestones {
			rewards, err := s.adminTypedResourceSpec(m.Rewards, true)
			if err != nil {
				return err
			}
			day := p4SignDay{Rewards: rewards}
			for _, option := range m.Options {
				spec, err := s.adminTypedResourceSpec([]model.AdminResourceAmount{option}, false)
				if err != nil {
					return err
				}
				day.Options = append(day.Options, spec)
			}
			rules.Days = append(rules.Days, day)
		}
		for _, m := range f.Options.Meals {
			if m.Amount < 0 || m.Amount > math.MaxInt32 {
				return constUtil.ErrAdminInput
			}
			rules.Meals = append(rules.Meals, p4Meal{StartMinute: m.StartMinute, EndMinute: m.EndMinute, Amount: int32(m.Amount)})
		}
		for _, v := range f.Options.CoinPrizes {
			if v.Coin < 1 || v.Coin > math.MaxInt32 {
				return constUtil.ErrAdminInput
			}
			rules.CatPrizes = append(rules.CatPrizes, p4CatPrize{Coin: int32(v.Coin), Weight: v.Weight})
		}
		var err error
		rules.ExploreReward, err = s.adminTypedResourceSpec(f.Options.ExploreRewards, true)
		if err != nil {
			return err
		}
		rules.FinalBonus, err = s.adminTypedResourceSpec(f.Options.FinalRewards, true)
		if err != nil {
			return err
		}
		cfg := db.P4WelfareRule{ID: key, Kind: d.kind, Enabled: enabled, StartAt: f.StartAt, EndAt: f.EndAt, ClaimEndAt: f.ClaimEndAt, Rules: adminJSON(rules), Source: f.Source}
		if enabled {
			if _, err := s.validateP4Welfare(cfg); err != nil {
				return adminActivityValidationError(err)
			}
		}
		return save(&cfg)
	case "period":
		if len(f.Milestones) > 1 || enabled && len(f.Milestones) != 1 {
			return constUtil.ErrAdminInput
		}
		goal := int64(1)
		rewards := ""
		if len(f.Milestones) > 0 {
			goal = f.Milestones[0].Goal
			var err error
			rewards, err = s.adminTypedResourceSpec(f.Milestones[0].Rewards, !enabled)
			if err != nil {
				return err
			}
		}
		cfg := db.PeriodInstance{ID: key, Kind: d.kind, Mode: "cumulative", Enabled: enabled, Title: f.Name, Goal: goal, Rewards: rewards, StartAt: f.StartAt, EndAt: f.EndAt, RewardEndAt: f.ClaimEndAt, Source: f.Source}
		if d.mode == "trial-stars" || d.mode == "power-race" {
			cfg.Mode = "single"
		}
		if f.Options.RankPlaces < 0 || f.Options.RankPlaces > 1000 || d.mode != "power-race" && (f.Options.RankPlaces != 0 || len(f.Options.RankRewards) != 0) {
			return constUtil.ErrAdminInput
		}
		if d.mode == "power-race" {
			cfg.RankPlaces = int32(f.Options.RankPlaces)
			var err error
			cfg.RankRewards, err = s.adminTypedResourceSpec(f.Options.RankRewards, !enabled || cfg.RankPlaces == 0)
			if err != nil {
				return err
			}
		}
		if enabled || rewards != "" {
			if err := s.validateP1Period(cfg); err != nil {
				return adminActivityValidationError(err)
			}
		}
		return save(&cfg)
	case "pool":
		cfg := db.GameplayPool{ID: key, Name: f.Name, Source: f.Source, Enabled: true, MinLevel: 1, SingleCost: "diamond,50", TenCost: "diamond,488", FreeSeconds: 86400, FreePerDay: 0, CoinPerDraw: 10000, StartAt: f.StartAt, EndAt: f.EndAt, RewardEndAt: f.ClaimEndAt, InitialDiamondPool: f.Options.InitialDiamondPool, JackpotPerMillion: int32(f.Options.JackpotPerMillion), PoolContributionBps: int32(f.Options.PoolContributionBps)}
		rewards := []db.GameplayPoolReward{}
		seen := map[int64]bool{}
		for _, v := range f.Prizes {
			if seen[v.ID] {
				return constUtil.ErrAdminInput
			}
			seen[v.ID] = true
			if v.ID < 0 || v.ID > 8 || v.Amount < 0 || v.Amount > math.MaxInt32 || v.Stock < 0 || v.Stock > math.MaxInt32 || v.Weight < 0 || v.Weight > 1000000000000 {
				return constUtil.ErrAdminInput
			}
			if v.ResourceID == "" && v.Amount == 0 && !enabled {
				continue
			}
			rewards = append(rewards, db.GameplayPoolReward{PoolID: key, ID: int32(v.ID), ResourceID: v.ResourceID, Amount: int32(v.Amount), Stock: int32(v.Stock), Weight: v.Weight, Guaranteed: v.Guaranteed})
		}
		top := ""
		advance := ""
		if f.Options.TopPrize.ResourceID != "" {
			top = fmt.Sprintf("%s,%d", f.Options.TopPrize.ResourceID, f.Options.TopPrize.Amount)
		}
		if f.Options.AdvancePrize.ResourceID != "" {
			advance = fmt.Sprintf("%s,%d", f.Options.AdvancePrize.ResourceID, f.Options.AdvancePrize.Amount)
		}
		wheel := db.TurntableRules{PoolID: key, ScorePerDraw: int32(f.Options.ScorePerDraw), TopPrize: top, AdvancePrize: advance, AdvancePlaces: int32(f.Options.AdvancePlaces), Description: f.Source}
		if err := save(&cfg); err != nil {
			return err
		}
		if exists {
			if err := tx.Where("pool_id=?", key).Delete(&db.GameplayPoolReward{}).Error; err != nil {
				return err
			}
		}
		if len(rewards) > 0 {
			if err := tx.Create(&rewards).Error; err != nil {
				return err
			}
		}
		if exists {
			if err := tx.Save(&wheel).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Create(&wheel).Error; err != nil {
				return err
			}
		}
		if enabled {
			if _, _, _, err := s.turntableConfigForPool(tx, key); err != nil {
				return adminActivityValidationError(err)
			}
		}
		return tx.Model(&db.GameplayPool{}).Where("id=?", key).Update("enabled", enabled).Error
	}
	return constUtil.ErrAdminInput
}

// AdminTypedActivityState 支持冻结活动启停，显式新转盘源只影响未来全局期。
func (s *GameService) AdminTypedActivityState(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminActivityStateRequest) (*model.AdminActivityInstance, error) {
	var saved model.AdminActivityInstance
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		raw, frozen, err := adminActivityRaw(tx.Clauses(clause.Locking{Strength: "UPDATE"}), id)
		if err != nil {
			return err
		}
		current, err := s.adminActivityTypedProjection(tx, raw, frozen)
		if err != nil {
			return err
		}
		if current.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Enabled && !current.Availability.Usable {
			return constUtil.ErrAdminInput
		}
		domain, key, _ := strings.Cut(id, ":")
		if req.Enabled && !frozen {
			d, ok := adminActivityDefinitionFor(current.Mode)
			if !ok {
				return constUtil.ErrAdminInput
			}
			if err := s.adminPersistTypedActivity(tx, d, key, current.Form, true, true); err != nil {
				return err
			}
		}
		table := map[string]string{"commerce": "p4_commerce_rules", "welfare": "p4_welfare_rules", "period": "p1_period_instances", "pool": "gameplay_pools"}[domain]
		if table == "" {
			return constUtil.ErrAdminInput
		}
		if err := tx.Table(table).Where("id=?", key).Update("enabled", req.Enabled).Error; err != nil {
			return err
		}
		if req.MakeCurrent {
			if !req.Enabled || current.Mode != "turntable" {
				return constUtil.ErrAdminInput
			}
			var king db.KingRecruitRules
			if err := tx.First(&king, 1).Error; err != nil {
				return err
			}
			if err := tx.Model(&king).Update("wheel_pool", key).Error; err != nil {
				return err
			}
		}
		raw, frozen, err = adminActivityRaw(tx, id)
		if err != nil {
			return err
		}
		saved, err = s.adminActivityTypedProjection(tx, raw, frozen)
		if err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.state", id, fmt.Sprintf("enabled=%t current=%t", req.Enabled, req.MakeCurrent))
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// AdminDeleteTypedActivity 归档入口并关闭后续参与，已有快照、回执和领奖历史全部保留。
func (s *GameService) AdminDeleteTypedActivity(ctx context.Context, p *model.AdminPrincipal, id, version string) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		raw, frozen, err := adminActivityRaw(tx.Clauses(clause.Locking{Strength: "UPDATE"}), id)
		if err != nil {
			return err
		}
		current, err := s.adminActivityTypedProjection(tx, raw, frozen)
		if err != nil {
			return err
		}
		if current.Version != version {
			return constUtil.ErrAdminConfigConflict
		}
		domain, key, _ := strings.Cut(id, ":")
		table := map[string]string{"commerce": "p4_commerce_rules", "welfare": "p4_welfare_rules", "period": "p1_period_instances", "pool": "gameplay_pools"}[domain]
		if table == "" {
			return constUtil.ErrAdminInput
		}
		if err := tx.Table(table).Where("id=?", key).Update("enabled", false).Error; err != nil {
			return err
		}
		meta := db.AdminActivityPresentation{ActivityID: id, Mode: current.Mode, TemplateID: current.TemplateID, Name: current.Name, Archived: true, UpdatedAtMillis: time.Now().UnixMilli()}
		if err := tx.Save(&meta).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.archive", id, "history preserved")
	})
}

// AdminCurrentWelfareRuleKey 让没有个人绑定的原无实例协议找到当前有效的运营福利期。
// 调用方应先保留既有localPlayWelfareRuleID结果；当前个人期继续使用自己的冻结规则。
func AdminCurrentWelfareRuleKey(tx *gorm.DB, kind string, now int64, fallback string) (string, error) {
	if !tx.Migrator().HasTable(&db.AdminActivityPresentation{}) {
		return adminActivityScheduleFallback(tx, fallback)
	}
	var meta []db.AdminActivityPresentation
	if err := tx.Where("archived=?", false).Find(&meta).Error; err != nil {
		return "", err
	}
	keys := []string{}
	for _, m := range meta {
		d, ok := adminActivityDefinitionFor(m.Mode)
		if ok && d.domain == "welfare" && d.kind == kind {
			_, key, _ := strings.Cut(m.ActivityID, ":")
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return adminActivityScheduleFallback(tx, fallback)
	}
	var rows []db.P4WelfareRule
	if err := tx.Where("id IN ? AND kind=? AND enabled=? AND start_at<=? AND claim_end_at>?", keys, kind, true, now, now).Order("start_at DESC,id DESC").Find(&rows).Error; err != nil {
		return "", err
	}
	for _, row := range rows {
		allowed, err := localPlayActivityVisible(tx, row.ID)
		if err != nil {
			return "", err
		}
		if allowed {
			return row.ID, nil
		}
	}
	return adminActivityScheduleFallback(tx, fallback)
}

// AdminCurrentCommerceRuleKey 让原神秘小屋等无实例查询选择已启用运营期，保持个人期优先。
func AdminCurrentCommerceRuleKey(tx *gorm.DB, kind string, now int64, fallback string) (string, error) {
	if !tx.Migrator().HasTable(&db.AdminActivityPresentation{}) {
		return adminActivityScheduleFallback(tx, fallback)
	}
	var meta []db.AdminActivityPresentation
	if err := tx.Where("archived=?", false).Find(&meta).Error; err != nil {
		return "", err
	}
	keys := []string{}
	for _, m := range meta {
		d, ok := adminActivityDefinitionFor(m.Mode)
		if ok && d.domain == "commerce" && d.kind == kind {
			_, key, _ := strings.Cut(m.ActivityID, ":")
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return adminActivityScheduleFallback(tx, fallback)
	}
	var rows []db.P4CommerceRule
	if err := tx.Where("id IN ? AND kind=? AND enabled=? AND start_at<=? AND end_at>?", keys, kind, true, now, now).Order("start_at DESC,id DESC").Find(&rows).Error; err != nil {
		return "", err
	}
	for _, row := range rows {
		allowed, err := localPlayActivityVisible(tx, row.ID)
		if err != nil {
			return "", err
		}
		if allowed {
			return row.ID, nil
		}
	}
	return adminActivityScheduleFallback(tx, fallback)
}

// AdminActivityDisplayName 只覆盖运营新实例标题，不更换原UI prefab或原协议ID。
func AdminActivityDisplayName(tx *gorm.DB, id, fallback string) string {
	if !tx.Migrator().HasTable(&db.AdminActivityPresentation{}) {
		return fallback
	}
	var row db.AdminActivityPresentation
	if tx.Where("activity_id=? AND archived=?", id, false).Take(&row).Error == nil && row.Name != "" {
		return row.Name
	}
	return fallback
}

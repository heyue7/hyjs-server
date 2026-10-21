package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

// InitializeAdminNativeActivitySettings 只迁移精彩活动自有策略表，其他配置仍由原消费者表承载。
func InitializeAdminNativeActivitySettings(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminNativeActivityPolicy{})
}

// adminNativeSnapshot 保存一次读取到的真实配置行，保存时只修改当前类型的既有行。
type adminNativeSnapshot struct {
	dto          model.AdminNativeActivityDTO
	fund         db.ActivityRules
	fundRewards  []db.ActivityReward
	payment      db.PaymentActivityRule
	product      db.PaymentProduct
	policy       db.AdminNativeActivityPolicy
	policyExists bool
	copyRule     db.P4CopyBenefitRule
	rotation     db.P4RecruitRotation
	firstDouble  db.FirstDoublePolicy
	period       db.PeriodInstance
	bundle       db.P15SeasonalBundle
	children     []p15SeasonalChild
	childForms   []model.AdminActivityForm
	childFrozen  []bool
}

func adminNativeKindName(kind string) string {
	switch kind {
	case "fund":
		return "开服基金"
	case "payment-first":
		return "首充礼包"
	case "payment-single":
		return "单充送礼"
	case "payment-total":
		return "累计充值"
	case "payment-continued":
		return "连充送礼"
	case "payment-consume":
		return "累计消费"
	case "month-card":
		return "月卡奖励"
	case "permanent-card":
		return "终身卡奖励"
	case "vip-benefits":
		return "会员福利"
	case "copy-benefits":
		return "福利活动"
	case "recruit-rotation":
		return "橙卡招募"
	case "first-double":
		return "首充双倍"
	case "celebration":
		return "庆典活动"
	case "festival":
		return "节日活动"
	case "update":
		return "新版活动"
	}
	return ""
}

func adminNativePaymentKind(ruleKind string) (string, bool) {
	switch ruleKind {
	case "first":
		return "payment-first", true
	case "single":
		return "payment-single", true
	case "total":
		return "payment-total", true
	case "continued":
		return "payment-continued", true
	case "consume":
		return "payment-consume", true
	}
	return "", false
}

func adminNativeProductKind(productKind string) (string, bool) {
	switch productKind {
	case "month":
		return "month-card", true
	case "permanent":
		return "permanent-card", true
	}
	return "", false
}

// adminNativeVersion 将实际生效表单、开关、别名和冻结状态一起纳入乐观锁。
func adminNativeVersion(form model.AdminNativeActivityForm, enabled bool, alias string, frozen bool) string {
	return adminVersion(struct {
		Form    model.AdminNativeActivityForm
		Enabled bool
		Alias   string
		Frozen  bool
	}{adminNativeNormalizeForm(form), enabled, alias, frozen})
}

func adminNativeNormalizeResources(values []model.AdminResourceAmount) []model.AdminResourceAmount {
	if values == nil {
		return []model.AdminResourceAmount{}
	}
	out := append([]model.AdminResourceAmount{}, values...)
	return out
}

func adminNativeNormalizeActivityForm(form model.AdminActivityForm) model.AdminActivityForm {
	if form.Products == nil {
		form.Products = []model.AdminActivityProduct{}
	}
	for i := range form.Products {
		form.Products[i].Rewards = adminNativeNormalizeResources(form.Products[i].Rewards)
	}
	if form.Milestones == nil {
		form.Milestones = []model.AdminActivityMilestone{}
	}
	for i := range form.Milestones {
		form.Milestones[i].Rewards = adminNativeNormalizeResources(form.Milestones[i].Rewards)
		form.Milestones[i].Options = adminNativeNormalizeResources(form.Milestones[i].Options)
	}
	if form.Prizes == nil {
		form.Prizes = []model.AdminActivityPrize{}
	}
	if form.Options.RankRewards == nil {
		form.Options.RankRewards = []model.AdminResourceAmount{}
	}
	if form.Options.ExploreRewards == nil {
		form.Options.ExploreRewards = []model.AdminResourceAmount{}
	}
	if form.Options.FinalRewards == nil {
		form.Options.FinalRewards = []model.AdminResourceAmount{}
	}
	if form.Options.Meals == nil {
		form.Options.Meals = []model.AdminMealWindow{}
	}
	if form.Options.CoinPrizes == nil {
		form.Options.CoinPrizes = []model.AdminCoinPrize{}
	}
	if form.Options.Discounts == nil {
		form.Options.Discounts = []int32{}
	}
	return form
}

// adminNativeNormalizeForm 统一nil与空数组，避免前端省略空字段被误判为规则变化。
func adminNativeNormalizeForm(form model.AdminNativeActivityForm) model.AdminNativeActivityForm {
	if form.Rewards == nil {
		form.Rewards = []model.AdminResourceAmount{}
	}
	if form.Milestones == nil {
		form.Milestones = []model.AdminNativeActivityMilestone{}
	}
	for i := range form.Milestones {
		form.Milestones[i].Rewards = adminNativeNormalizeResources(form.Milestones[i].Rewards)
	}
	if form.Prizes == nil {
		form.Prizes = []model.AdminActivityPrize{}
	}
	if form.VipTiers == nil {
		form.VipTiers = []model.AdminNativeVIPTier{}
	}
	for i := range form.VipTiers {
		form.VipTiers[i].DailyRewards = adminNativeNormalizeResources(form.VipTiers[i].DailyRewards)
		form.VipTiers[i].WeekRewards = adminNativeNormalizeResources(form.VipTiers[i].WeekRewards)
	}
	if form.Children == nil {
		form.Children = []model.AdminNativeActivityChild{}
	}
	for i := range form.Children {
		form.Children[i].Form = adminNativeNormalizeActivityForm(form.Children[i].Form)
	}
	return form
}

// adminNativeSameForm 逐字段比较实际提交与数据库规则，不能只相信前端携带的版本。
func adminNativeSameForm(a, b model.AdminNativeActivityForm) bool {
	return adminVersion(adminNativeNormalizeForm(a)) == adminVersion(adminNativeNormalizeForm(b))
}

func adminNativeLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return constUtil.ErrAdminInput
	}
	return err
}

func adminNativeMeta(tx *gorm.DB, id string, lock bool) (db.AdminActivityPresentation, error) {
	var meta db.AdminActivityPresentation
	if !tx.Migrator().HasTable(&db.AdminActivityPresentation{}) {
		return meta, nil
	}
	query := tx.Where("activity_id = ?", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&meta).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.AdminActivityPresentation{}, nil
	}
	return meta, err
}

func adminNativeSavePresentation(tx *gorm.DB, id, kind, alias string) error {
	if !tx.Migrator().HasTable(&db.AdminActivityPresentation{}) {
		return constUtil.ErrAdminInput
	}
	meta := db.AdminActivityPresentation{ActivityID: id, Mode: "native-" + kind, TemplateID: "native-" + kind, Name: adminNativeKindName(kind), Alias: alias, UpdatedAtMillis: time.Now().UnixMilli()}
	return tx.Save(&meta).Error
}

func adminNativeChildLabel(domain string) string {
	if domain == "period" {
		return "登录奖励"
	}
	return "限时兑换"
}

// adminNativeLoad 按配置ID读取真实来源；lock时按既有消费者顺序取得必要行锁。
func (s *GameService) adminNativeLoad(tx *gorm.DB, id string, lock bool) (*adminNativeSnapshot, error) {
	snap := &adminNativeSnapshot{}
	snap.dto.ID = id
	switch {
	case id == "vip-benefits":
		form, enabled, exists, err := s.adminNativeVIPPolicy(tx, lock)
		if err != nil {
			return nil, err
		}
		snap.dto.Kind = "vip-benefits"
		snap.dto.Form = form
		snap.dto.Enabled = enabled
		snap.dto.Frozen = false
		snap.dto.Note = "会员福利；资格仍为真实确认订单且VIP>0，领取键与每日/每周周期不变；关闭后显示不可领并拒绝领取。"
		snap.policyExists = exists
		if exists {
			var policy db.AdminNativeActivityPolicy
			if err := tx.First(&policy, "id = ?", "vip-benefits").Error; err != nil {
				return nil, err
			}
			snap.policy = policy
		}
	case id == "first-double":
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&snap.firstDouble, "id = ?", 1).Error; err != nil {
			return nil, adminNativeLookupError(err)
		}
		snap.dto.Kind = "first-double"
		snap.dto.Form = model.AdminNativeActivityForm{}
		snap.dto.Enabled = snap.firstDouble.Enabled
		snap.dto.Frozen = true
		snap.dto.Note = "仅启停和别名；原SKU终身首单资格与已生成订单保持，不重置终身历史。"
	case strings.HasPrefix(id, "fund:"):
		key := strings.TrimPrefix(id, "fund:")
		if key != "open_server_fund" {
			return nil, constUtil.ErrAdminInput
		}
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&snap.fund, "id = ?", key).Error; err != nil {
			return nil, adminNativeLookupError(err)
		}
		rewards := tx.Where("activity_id = ?", key)
		if lock {
			rewards = rewards.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := rewards.Find(&snap.fundRewards).Error; err != nil {
			return nil, err
		}
		sort.SliceStable(snap.fundRewards, func(i, j int) bool {
			left, right := snap.fundRewards[i], snap.fundRewards[j]
			if left.RewardType != right.RewardType {
				return left.RewardType == "openServerFund"
			}
			return left.Threshold < right.Threshold
		})
		form := model.AdminNativeActivityForm{StartAt: snap.fund.StartAt, EndAt: snap.fund.EndAt, ClaimEndAt: snap.fund.RewardEndAt, MinVip: int64(snap.fund.MinVIP), DiamondCost: int64(snap.fund.DiamondCost), Milestones: []model.AdminNativeActivityMilestone{}}
		for _, row := range snap.fundRewards {
			form.Milestones = append(form.Milestones, model.AdminNativeActivityMilestone{Key: row.RewardType + ":" + strconv.Itoa(int(row.Threshold)), Goal: int64(activityRewardDisplayIndex(row)), Rewards: adminTypedResources(row.Reward)})
		}
		snap.dto.Kind = "fund"
		snap.dto.Form = form
		snap.dto.Enabled = snap.fund.Enabled
		snap.dto.Frozen = false
		snap.dto.Note = "开服基金；含全民福利。已领取档位保留，未领取档位按新配置沿当前消费者发放。"
	case strings.HasPrefix(id, "payment:"):
		key := strings.TrimPrefix(id, "payment:")
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&snap.payment, "id = ?", key).Error; err != nil {
			return nil, adminNativeLookupError(err)
		}
		kind, ok := adminNativePaymentKind(snap.payment.Kind)
		if !ok {
			return nil, constUtil.ErrAdminInput
		}
		snap.dto.Kind = kind
		snap.dto.Form = model.AdminNativeActivityForm{StartAt: snap.payment.StartAt, EndAt: snap.payment.EndAt, ClaimEndAt: snap.payment.RewardEndAt, Threshold: snap.payment.Threshold, Rewards: adminTypedResources(snap.payment.Reward)}
		snap.dto.Enabled = snap.payment.Enabled
		snap.dto.Frozen = false
		snap.dto.Note = "支付活动；金额单位为人民币分，累计消费为钻石、连充为天数。仅影响尚未首次观察的玩家，已观察玩家继续使用首次观察冻结快照和领取记录。"
	case strings.HasPrefix(id, "product:"):
		key := strings.TrimPrefix(id, "product:")
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&snap.product, "id = ?", key).Error; err != nil {
			return nil, adminNativeLookupError(err)
		}
		kind, ok := adminNativeProductKind(snap.product.Kind)
		if !ok {
			return nil, constUtil.ErrAdminInput
		}
		snap.dto.Kind = kind
		snap.dto.Form = model.AdminNativeActivityForm{Cents: snap.product.Cents, Charge: snap.product.Charge, DailyDiamond: snap.product.Free, Rewards: adminTypedResources(snap.product.Reward)}
		snap.dto.Enabled = snap.product.Enabled
		snap.dto.Frozen = false
		snap.dto.Note = "卡类奖励；只修改价格、充值额度、每日钻石、附带奖励和开关。已确认与待支付订单快照不变，修改仅影响新订单。"
	case strings.HasPrefix(id, "copy:"):
		key := strings.TrimPrefix(id, "copy:")
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&snap.copyRule, "id = ?", key).Error; err != nil {
			return nil, adminNativeLookupError(err)
		}
		effective, err := adminNativeCopyEffective(snap.copyRule)
		if err != nil {
			return nil, err
		}
		snap.dto.Kind = "copy-benefits"
		snap.dto.Form = model.AdminNativeActivityForm{StartAt: effective.StartAt, EndAt: effective.EndAt, WeekdayMask: int64(effective.WeekdayMask), StartMinute: int64(effective.StartMinute), EndMinute: int64(effective.EndMinute), CurrencyFactor: int64(effective.CurrencyFactor), DropFactor: int64(effective.DropFactor)}
		snap.dto.Enabled = snap.copyRule.Enabled
		snap.dto.Frozen = len(snap.copyRule.Frozen) > 0
		snap.dto.Note = "副本福利；未冻结保存立即写入原表并生效，冻结后仅允许启停和别名。"
	case strings.HasPrefix(id, "recruit:"):
		key := strings.TrimPrefix(id, "recruit:")
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&snap.rotation, "id = ?", key).Error; err != nil {
			return nil, adminNativeLookupError(err)
		}
		if snap.rotation.Kind != 2 {
			return nil, constUtil.ErrAdminInput
		}
		raw := snap.rotation.Frozen
		if len(raw) == 0 {
			raw = snap.rotation.Rewards
		}
		var rewards []db.GameplayPoolReward
		if err := json.Unmarshal(raw, &rewards); err != nil {
			return nil, constUtil.ErrAdminInput
		}
		form := model.AdminNativeActivityForm{StartAt: snap.rotation.StartAt, EndAt: snap.rotation.EndAt, Prizes: []model.AdminActivityPrize{}}
		for _, reward := range rewards {
			form.Prizes = append(form.Prizes, model.AdminActivityPrize{ID: int64(reward.ID), ResourceID: reward.ResourceID, Amount: int64(reward.Amount), Weight: reward.Weight, Stock: int64(reward.Stock), Guaranteed: reward.Guaranteed})
		}
		snap.dto.Kind = "recruit-rotation"
		snap.dto.Form = form
		snap.dto.Enabled = snap.rotation.Enabled
		snap.dto.Frozen = len(snap.rotation.Frozen) > 0
		snap.dto.Note = "橙卡轮换奖池与开放时间；费用和免费冷却沿用原规则。冻结后仅允许启停、别名和全局招募规则。"
	case strings.HasPrefix(id, "celebration:"):
		key := strings.TrimPrefix(id, "celebration:")
		if !strings.HasPrefix(key, "celebrationActivity_") {
			return nil, constUtil.ErrAdminInput
		}
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&snap.period, "id = ?", key).Error; err != nil {
			return nil, adminNativeLookupError(err)
		}
		effective, err := adminNativePeriodEffective(snap.period)
		if err != nil {
			return nil, err
		}
		form := model.AdminNativeActivityForm{StartAt: effective.StartAt, EndAt: effective.EndAt, ClaimEndAt: effective.RewardEndAt, Milestones: []model.AdminNativeActivityMilestone{}}
		for index, tier := range p1PeriodMilestones(effective) {
			form.Milestones = append(form.Milestones, model.AdminNativeActivityMilestone{Key: "tier:" + strconv.Itoa(index+1), Goal: tier.Goal, Rewards: adminTypedResources(tier.Rewards)})
		}
		snap.dto.Kind = "celebration"
		snap.dto.Form = form
		snap.dto.Enabled = snap.period.Enabled
		snap.dto.Frozen = len(snap.period.Frozen) > 0
		snap.dto.Note = "庆典活动；沿用既有P1周期和领奖消费者，冻结后仅允许启停和别名。"
	case strings.HasPrefix(id, "p15:"):
		key := strings.TrimPrefix(id, "p15:")
		if !isP15SeasonalParent(key) {
			return nil, constUtil.ErrAdminInput
		}
		live, err := s.adminNativeLoadP15(tx, key, lock)
		if err != nil {
			return nil, err
		}
		effective, err := adminNativeP15Effective(live)
		if err != nil {
			return nil, err
		}
		var children []p15SeasonalChild
		if len(effective.Children) > 16384 || json.Unmarshal(effective.Children, &children) != nil {
			return nil, constUtil.ErrAdminInput
		}
		form := model.AdminNativeActivityForm{StartAt: effective.StartAt, EndAt: effective.EndAt, ClaimEndAt: effective.RewardEndAt, Children: []model.AdminNativeActivityChild{}}
		frozen := len(live.Frozen) > 0
		for _, child := range children {
			raw, childFrozen, err := adminActivityRaw(tx, child.Domain+":"+child.ID)
			if err != nil {
				return nil, adminNativeLookupError(err)
			}
			childDTO, err := s.adminActivityTypedProjection(tx, raw, childFrozen)
			if err != nil {
				return nil, err
			}
			childForm := childDTO.Form
			childForm.Name = adminNativeChildLabel(child.Domain)
			form.Children = append(form.Children, model.AdminNativeActivityChild{ID: child.ID, Domain: child.Domain, Form: childForm})
			snap.children = append(snap.children, child)
			snap.childForms = append(snap.childForms, childForm)
			snap.childFrozen = append(snap.childFrozen, childFrozen)
			frozen = frozen || childFrozen
		}
		snap.bundle = live
		snap.dto.Kind = live.Kind
		snap.dto.Form = form
		snap.dto.Enabled = live.Enabled
		snap.dto.Frozen = frozen
		snap.dto.Note = "节日/新版活动；子目录和域名固定，父子时间统一覆盖，全部完整才可启用。父或任一子冻结后仅允许启停和别名，启停父与已有子同步。"
	default:
		return nil, constUtil.ErrAdminInput
	}
	if snap.dto.Kind == "" {
		return nil, constUtil.ErrAdminInput
	}
	meta, err := adminNativeMeta(tx, id, lock)
	if err != nil {
		return nil, err
	}
	snap.dto.Name = adminNativeKindName(snap.dto.Kind)
	snap.dto.Alias = meta.Alias
	snap.dto.Version = adminNativeVersion(snap.dto.Form, snap.dto.Enabled, snap.dto.Alias, snap.dto.Frozen)
	return snap, nil
}

// adminNativeLoadP15 先按周期→父目录→商品顺序加锁，避免与领奖和兑换路径形成反向锁序。
func (s *GameService) adminNativeLoadP15(tx *gorm.DB, key string, lock bool) (db.P15SeasonalBundle, error) {
	var live db.P15SeasonalBundle
	if !lock {
		err := tx.First(&live, "id = ?", key).Error
		return live, adminNativeLookupError(err)
	}
	var pre db.P15SeasonalBundle
	if err := tx.First(&pre, "id = ?", key).Error; err != nil {
		return live, adminNativeLookupError(err)
	}
	var preChildren []p15SeasonalChild
	if len(pre.Children) > 16384 || json.Unmarshal(pre.Children, &preChildren) != nil {
		return live, constUtil.ErrAdminInput
	}
	periodIDs := []string{}
	for _, child := range preChildren {
		if child.Domain == "period" {
			periodIDs = append(periodIDs, child.ID)
		}
	}
	sort.Strings(periodIDs)
	for _, id := range periodIDs {
		var row db.PeriodInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", id).Error; err != nil {
			return live, adminNativeLookupError(err)
		}
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id = ?", key).Error; err != nil {
		return live, adminNativeLookupError(err)
	}
	effective, err := adminNativeP15Effective(live)
	if err != nil {
		return live, err
	}
	var children []p15SeasonalChild
	if len(effective.Children) > 16384 || json.Unmarshal(effective.Children, &children) != nil {
		return live, constUtil.ErrAdminInput
	}
	if !adminNativeSameChildren(preChildren, children) {
		return live, constUtil.ErrAdminConfigConflict
	}
	commerceIDs := []string{}
	for _, child := range children {
		if child.Domain == "commerce" {
			commerceIDs = append(commerceIDs, child.ID)
		}
	}
	sort.Strings(commerceIDs)
	for _, id := range commerceIDs {
		var row db.P4CommerceRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", id).Error; err != nil {
			return live, adminNativeLookupError(err)
		}
	}
	return live, nil
}

func adminNativeSameChildren(a, b []p15SeasonalChild) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Domain != b[i].Domain {
			return false
		}
	}
	return true
}

func adminNativeCopyEffective(live db.P4CopyBenefitRule) (db.P4CopyBenefitRule, error) {
	cfg := live
	if len(live.Frozen) > 0 {
		if json.Unmarshal(live.Frozen, &cfg) != nil || cfg.ID != live.ID {
			return cfg, constUtil.ErrAdminInput
		}
		cfg.Enabled = live.Enabled
	}
	if err := validateP4CopyBenefit(cfg); err != nil {
		return cfg, constUtil.ErrAdminInput
	}
	return cfg, nil
}

func adminNativePeriodEffective(live db.PeriodInstance) (db.PeriodInstance, error) {
	cfg := live
	if len(live.Frozen) > 0 {
		if json.Unmarshal(live.Frozen, &cfg) != nil || cfg.ID != live.ID {
			return cfg, constUtil.ErrAdminInput
		}
		cfg.Enabled = live.Enabled
	}
	return cfg, nil
}

func adminNativeP15Effective(live db.P15SeasonalBundle) (db.P15SeasonalBundle, error) {
	cfg := live
	if len(live.Frozen) > 0 {
		if len(live.Frozen) > 65536 || json.Unmarshal(live.Frozen, &cfg) != nil || cfg.ID != live.ID {
			return cfg, constUtil.ErrAdminInput
		}
		cfg.Enabled = live.Enabled
	}
	return cfg, nil
}

// AdminNativeActivities 列出九类真实后台配置；没有真实来源的ID不会凭空出现。
func (s *GameService) AdminNativeActivities(ctx context.Context, p *model.AdminPrincipal) ([]model.AdminNativeActivityDTO, error) {
	tx := db.GetIns().WithContext(ctx)
	// 列表与保存使用同一权限口径，读取也要求有效后台账号。
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	ids := []string{}
	var count int64
	if tx.Migrator().HasTable(&db.ActivityRules{}) {
		if err := tx.Model(&db.ActivityRules{}).Where("id = ?", "open_server_fund").Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			ids = append(ids, "fund:open_server_fund")
		}
	}
	if tx.Migrator().HasTable(&db.PaymentActivityRule{}) {
		var payments []db.PaymentActivityRule
		if err := tx.Order("id").Find(&payments).Error; err != nil {
			return nil, err
		}
		for _, row := range payments {
			if _, ok := adminNativePaymentKind(row.Kind); ok {
				ids = append(ids, "payment:"+row.ID)
			}
		}
	}
	if tx.Migrator().HasTable(&db.PaymentProduct{}) {
		var products []db.PaymentProduct
		if err := tx.Where("kind IN ?", []string{"month", "permanent"}).Order("id").Find(&products).Error; err != nil {
			return nil, err
		}
		for _, row := range products {
			// 当前客户端固定映射 gf_default；其他渠道保留支付兼容和订单历史，不占当前活动列表。
			for _, channel := range strings.Split(row.Channels, ";") {
				if channel == p3PaymentChannel("AND0003") {
					ids = append(ids, "product:"+row.ID)
					break
				}
			}
		}
	}
	ids = append(ids, "vip-benefits")
	if tx.Migrator().HasTable(&db.P4CopyBenefitRule{}) {
		var copies []db.P4CopyBenefitRule
		if err := tx.Order("id").Find(&copies).Error; err != nil {
			return nil, err
		}
		for _, row := range copies {
			ids = append(ids, "copy:"+row.ID)
		}
	}
	if tx.Migrator().HasTable(&db.P4RecruitRotation{}) {
		var rotations []db.P4RecruitRotation
		if err := tx.Where("kind = ?", 2).Order("id").Find(&rotations).Error; err != nil {
			return nil, err
		}
		for _, row := range rotations {
			ids = append(ids, "recruit:"+row.ID)
		}
	}
	if tx.Migrator().HasTable(&db.FirstDoublePolicy{}) {
		if err := tx.Model(&db.FirstDoublePolicy{}).Where("id = ?", 1).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			ids = append(ids, "first-double")
		}
	}
	if tx.Migrator().HasTable(&db.PeriodInstance{}) {
		var periods []db.PeriodInstance
		if err := tx.Where("id LIKE ?", "celebrationActivity%").Order("id").Find(&periods).Error; err != nil {
			return nil, err
		}
		for _, row := range periods {
			if strings.HasPrefix(row.ID, "celebrationActivity_") {
				ids = append(ids, "celebration:"+row.ID)
			}
		}
	}
	if tx.Migrator().HasTable(&db.P15SeasonalBundle{}) {
		var bundles []db.P15SeasonalBundle
		if err := tx.Order("id").Find(&bundles).Error; err != nil {
			return nil, err
		}
		for _, row := range bundles {
			if row.Kind == "festival" || row.Kind == "update" {
				ids = append(ids, "p15:"+row.ID)
			}
		}
	}
	// 一次读取移除标记，原配置和所有奖励历史继续留在各业务表。
	archived := map[string]bool{}
	if tx.Migrator().HasTable(&db.AdminActivityPresentation{}) {
		var hidden []db.AdminActivityPresentation
		if err := tx.Select("activity_id").Where("archived = ?", true).Find(&hidden).Error; err != nil {
			return nil, err
		}
		for _, row := range hidden {
			archived[row.ActivityID] = true
		}
	}
	out := []model.AdminNativeActivityDTO{}
	for _, id := range ids {
		// 个人活动日历的独立期由个人工具管理，后台只配置公共来源。
		if archived[id] || strings.Contains(id, "_lp-") {
			continue
		}
		snap, err := s.adminNativeLoad(tx, id, false)
		if err != nil {
			return nil, err
		}
		out = append(out, snap.dto)
	}
	return out, nil
}

// AdminSaveNativeActivity 只修改已存在配置；冻结规则只能启停或改别名。
func (s *GameService) AdminSaveNativeActivity(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminNativeActivitySaveRequest) (*model.AdminNativeActivityDTO, error) {
	var saved model.AdminNativeActivityDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		snap, err := s.adminNativeLoad(tx, id, true)
		if err != nil {
			return err
		}
		meta, err := adminNativeMeta(tx, id, true)
		if err != nil {
			return err
		}
		if meta.Archived {
			return constUtil.ErrAdminConfigConflict
		}
		if snap.dto.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.RecruitSettings != nil {
			kind := int32(0)
			if snap.dto.Kind == "recruit-rotation" {
				kind = constUtil.RecruitRuntimeKindStrong
			}
			if err := s.adminSaveActivityRecruitSettings(tx, p, kind, req.RecruitSettings); err != nil {
				return err
			}
		}
		same := adminNativeSameForm(snap.dto.Form, req.Form)
		if snap.dto.Frozen {
			if !same {
				return constUtil.ErrAdminConfigConflict
			}
			if err := s.adminNativeApplyFrozenState(tx, snap, req.Enabled); err != nil {
				return err
			}
		} else if err := s.adminNativeApply(tx, snap, req); err != nil {
			return err
		}
		if err := adminNativeSavePresentation(tx, id, snap.dto.Kind, req.Alias); err != nil {
			return err
		}
		fresh, err := s.adminNativeLoad(tx, id, false)
		if err != nil {
			return err
		}
		saved = fresh.dto
		action := "native-activity.update"
		if same && snap.dto.Enabled != req.Enabled {
			action = "native-activity.state"
		}
		return adminAudit(tx, p, action, id, fmt.Sprintf("kind=%s enabled=%t version=%s", snap.dto.Kind, req.Enabled, snap.dto.Version))
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *GameService) adminNativeApply(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	switch snap.dto.Kind {
	case "fund":
		return s.adminNativeApplyFund(tx, snap, req)
	case "payment-first", "payment-single", "payment-total", "payment-continued", "payment-consume":
		return s.adminNativeApplyPayment(tx, snap, req)
	case "month-card", "permanent-card":
		return s.adminNativeApplyProduct(tx, snap, req)
	case "vip-benefits":
		return s.adminNativeApplyVIP(tx, snap, req)
	case "copy-benefits":
		return s.adminNativeApplyCopy(tx, snap, req)
	case "recruit-rotation":
		return s.adminNativeApplyRecruit(tx, snap, req)
	case "first-double":
		return s.adminNativeApplyFirstDouble(tx, snap, req)
	case "celebration":
		return s.adminNativeApplyCelebration(tx, snap, req)
	case "festival", "update":
		return s.adminNativeApplyP15(tx, snap, req)
	}
	return constUtil.ErrAdminInput
}

func (s *GameService) adminNativeApplyFund(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form := req.Form
	if form.StartAt <= 0 || form.EndAt <= form.StartAt || form.ClaimEndAt < form.EndAt || form.MinVip < 0 || form.MinVip > 12 || form.DiamondCost <= 0 || form.DiamondCost > math.MaxInt32 {
		return constUtil.ErrAdminInput
	}
	if len(snap.fundRewards) == 0 || len(form.Milestones) != len(snap.fundRewards) {
		return constUtil.ErrAdminInput
	}
	byKey := map[string]model.AdminNativeActivityMilestone{}
	goals := map[int64]bool{}
	for _, milestone := range form.Milestones {
		if milestone.Key == "" || byKey[milestone.Key].Key != "" || len(milestone.Key) > 64 {
			return constUtil.ErrAdminInput
		}
		byKey[milestone.Key] = milestone
	}
	for _, row := range snap.fundRewards {
		key := row.RewardType + ":" + strconv.Itoa(int(row.Threshold))
		milestone, ok := byKey[key]
		if !ok {
			return constUtil.ErrAdminInput
		}
		if row.RewardType == "openServerFund" {
			if milestone.Goal != int64(row.Threshold) {
				return constUtil.ErrAdminInput
			}
		} else if row.RewardType == "everyoneBenefit" {
			if milestone.Goal < 1 || milestone.Goal > math.MaxInt32 || goals[milestone.Goal] {
				return constUtil.ErrAdminInput
			}
			goals[milestone.Goal] = true
		} else {
			return constUtil.ErrAdminInput
		}
		spec, err := s.adminTypedResourceSpec(milestone.Rewards, !req.Enabled)
		if err != nil {
			return err
		}
		if req.Enabled && spec == "" {
			return constUtil.ErrAdminInput
		}
		updates := map[string]any{"reward": spec}
		if row.RewardType == "everyoneBenefit" {
			updates["required_count"] = int32(milestone.Goal)
		}
		if err := tx.Model(&db.ActivityReward{}).Where("activity_id = ? AND reward_type = ? AND threshold = ?", row.ActivityID, row.RewardType, row.Threshold).Updates(updates).Error; err != nil {
			return err
		}
	}
	return tx.Model(&db.ActivityRules{}).Where("id = ?", snap.fund.ID).Updates(map[string]any{
		"start_at":      form.StartAt,
		"end_at":        form.EndAt,
		"reward_end_at": form.ClaimEndAt,
		"min_vip":       int32(form.MinVip),
		"diamond_cost":  int32(form.DiamondCost),
		"enabled":       req.Enabled,
	}).Error
}

func (s *GameService) adminNativeApplyPayment(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form := req.Form
	if form.StartAt <= 0 || form.EndAt <= form.StartAt || form.ClaimEndAt < form.EndAt || form.Threshold <= 0 || form.Threshold > math.MaxInt32 {
		return constUtil.ErrAdminInput
	}
	spec, err := s.adminTypedResourceSpec(form.Rewards, !req.Enabled)
	if err != nil {
		return err
	}
	if req.Enabled && spec == "" {
		return constUtil.ErrAdminInput
	}
	updated := snap.payment
	updated.StartAt = form.StartAt
	updated.EndAt = form.EndAt
	updated.RewardEndAt = form.ClaimEndAt
	updated.Threshold = form.Threshold
	updated.Reward = spec
	updated.Enabled = req.Enabled
	if updated.StartAt >= updated.EndAt || updated.EndAt > updated.RewardEndAt || updated.Threshold <= 0 {
		return constUtil.ErrAdminInput
	}
	if _, err := p3Resources(updated.Reward); err != nil && req.Enabled {
		return constUtil.ErrAdminInput
	}
	return tx.Model(&db.PaymentActivityRule{}).Where("id = ?", updated.ID).Updates(map[string]any{
		"start_at":      updated.StartAt,
		"end_at":        updated.EndAt,
		"reward_end_at": updated.RewardEndAt,
		"threshold":     updated.Threshold,
		"reward":        updated.Reward,
		"enabled":       updated.Enabled,
	}).Error
}

func (s *GameService) adminNativeApplyProduct(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form := req.Form
	if form.Cents <= 0 || form.Cents > math.MaxInt32 || form.Charge < 0 || form.Charge > math.MaxInt32 || form.DailyDiamond < 0 || form.DailyDiamond > math.MaxInt32 {
		return constUtil.ErrAdminInput
	}
	if req.Enabled && form.DailyDiamond <= 0 {
		return constUtil.ErrAdminInput
	}
	spec, err := s.adminTypedResourceSpec(form.Rewards, true)
	if err != nil {
		return err
	}
	updated := snap.product
	updated.Cents = form.Cents
	updated.Charge = form.Charge
	updated.Free = form.DailyDiamond
	updated.Reward = spec
	updated.Enabled = req.Enabled
	if err := validateP3Product(&updated); err != nil {
		return constUtil.ErrAdminInput
	}
	return tx.Model(&db.PaymentProduct{}).Where("id = ?", updated.ID).Updates(map[string]any{
		"cents":   updated.Cents,
		"charge":  updated.Charge,
		"free":    updated.Free,
		"reward":  updated.Reward,
		"enabled": updated.Enabled,
	}).Error
}

func (s *GameService) adminNativeApplyVIP(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form, err := s.adminNativeValidateVIPTiers(req.Form, req.Enabled)
	if err != nil {
		return err
	}
	raw := adminJSON(form)
	version := adminNativeVersion(form, req.Enabled, req.Alias, false)
	policy := db.AdminNativeActivityPolicy{ID: "vip-benefits", Form: raw, Enabled: req.Enabled, Version: version, UpdatedAt: time.Now().UnixMilli()}
	return tx.Save(&policy).Error
}

func (s *GameService) adminNativeApplyCopy(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form := req.Form
	if form.StartAt <= 0 || form.EndAt <= form.StartAt || form.WeekdayMask < 1 || form.WeekdayMask > 127 || form.StartMinute < 0 || form.EndMinute < 1 || form.EndMinute > 1440 || form.StartMinute >= form.EndMinute || form.CurrencyFactor < 1 || form.CurrencyFactor > constUtil.P4CopyBenefitsMaximumFactor || form.DropFactor < 1 || form.DropFactor > constUtil.P4CopyBenefitsMaximumFactor {
		return constUtil.ErrAdminInput
	}
	updated := snap.copyRule
	updated.StartAt = form.StartAt
	updated.EndAt = form.EndAt
	updated.WeekdayMask = int32(form.WeekdayMask)
	updated.StartMinute = int32(form.StartMinute)
	updated.EndMinute = int32(form.EndMinute)
	updated.CurrencyFactor = int32(form.CurrencyFactor)
	updated.DropFactor = int32(form.DropFactor)
	updated.Enabled = req.Enabled
	if err := validateP4CopyBenefit(updated); err != nil {
		return constUtil.ErrAdminInput
	}
	return tx.Model(&db.P4CopyBenefitRule{}).Where("id = ?", updated.ID).Updates(map[string]any{
		"start_at":        updated.StartAt,
		"end_at":          updated.EndAt,
		"weekday_mask":    updated.WeekdayMask,
		"start_minute":    updated.StartMinute,
		"end_minute":      updated.EndMinute,
		"currency_factor": updated.CurrencyFactor,
		"drop_factor":     updated.DropFactor,
		"enabled":         updated.Enabled,
	}).Error
}

func (s *GameService) adminNativeApplyRecruit(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form := req.Form
	if form.StartAt <= 0 || form.EndAt <= form.StartAt || (len(form.Prizes) == 0 && req.Enabled) || len(form.Prizes) > 1000 {
		return constUtil.ErrAdminInput
	}
	rewards := []db.GameplayPoolReward{}
	seenIDs := map[int64]bool{}
	for _, prize := range form.Prizes {
		// 库存和单次数量由原招募奖池合同固定，后台只接收英雄、权重和原奖位ID。
		if prize.ID < 0 || prize.ID > math.MaxInt32 || seenIDs[prize.ID] || prize.ResourceID == "" || prize.Weight < 1 || prize.Weight > 1000000000000 {
			return constUtil.ErrAdminInput
		}
		seenIDs[prize.ID] = true
		guaranteed := false
		if hero := s.table("Hero", "id", prize.ResourceID); hero != nil {
			guaranteed = hero["quality"] == "20"
		}
		rewards = append(rewards, db.GameplayPoolReward{PoolID: "recruit_2", ID: int32(prize.ID), ResourceID: prize.ResourceID, Amount: 1, Stock: 0, Weight: prize.Weight, Guaranteed: guaranteed})
	}
	raw := adminJSON(rewards)
	updated := snap.rotation
	updated.StartAt = form.StartAt
	updated.EndAt = form.EndAt
	updated.Rewards = raw
	updated.Enabled = req.Enabled
	updated.Frozen = nil
	if _, _, err := s.p4RotationSnapshot(updated, 2); err != nil {
		return constUtil.ErrAdminInput
	}
	return tx.Model(&db.P4RecruitRotation{}).Where("id = ?", updated.ID).Updates(map[string]any{
		"start_at": updated.StartAt,
		"end_at":   updated.EndAt,
		"rewards":  updated.Rewards,
		"enabled":  updated.Enabled,
	}).Error
}

func (s *GameService) adminNativeApplyFirstDouble(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	if req.Enabled && (snap.firstDouble.Version != constUtil.FirstDoubleVersion || snap.firstDouble.Source != constUtil.FirstDoubleSource) {
		return constUtil.ErrAdminInput
	}
	return tx.Model(&db.FirstDoublePolicy{}).Where("id = ?", 1).Update("enabled", req.Enabled).Error
}

func (s *GameService) adminNativeApplyCelebration(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form := req.Form
	if form.StartAt <= 0 || form.EndAt <= form.StartAt || form.ClaimEndAt < form.EndAt || len(form.Milestones) < 1 || len(form.Milestones) > 31 {
		return constUtil.ErrAdminInput
	}
	tiers := []p1PeriodMilestone{}
	previous := int64(0)
	for _, milestone := range form.Milestones {
		if milestone.Goal <= previous || milestone.Goal > math.MaxInt32 {
			return constUtil.ErrAdminInput
		}
		spec, err := s.adminTypedResourceSpec(milestone.Rewards, false)
		if err != nil {
			return err
		}
		tiers = append(tiers, p1PeriodMilestone{Goal: milestone.Goal, Rewards: spec})
		previous = milestone.Goal
	}
	updated := snap.period
	updated.StartAt = form.StartAt
	updated.EndAt = form.EndAt
	updated.RewardEndAt = form.ClaimEndAt
	updated.Goal = tiers[0].Goal
	updated.Rewards = tiers[0].Rewards
	updated.Milestones = nil
	if len(tiers) > 1 {
		updated.Milestones = adminJSON(tiers)
	}
	updated.Enabled = req.Enabled
	if err := s.validateP1Period(updated); err != nil {
		return constUtil.ErrAdminInput
	}
	return tx.Model(&db.PeriodInstance{}).Where("id = ?", updated.ID).Updates(map[string]any{
		"start_at":      updated.StartAt,
		"end_at":        updated.EndAt,
		"reward_end_at": updated.RewardEndAt,
		"goal":          updated.Goal,
		"rewards":       updated.Rewards,
		"milestones":    updated.Milestones,
		"enabled":       updated.Enabled,
	}).Error
}

func (s *GameService) adminNativeApplyP15(tx *gorm.DB, snap *adminNativeSnapshot, req model.AdminNativeActivitySaveRequest) error {
	form := req.Form
	if form.StartAt <= 0 || form.EndAt <= form.StartAt || form.ClaimEndAt < form.EndAt || len(form.Children) != len(snap.children) || len(form.Children) < 1 {
		return constUtil.ErrAdminInput
	}
	incoming := map[string]model.AdminNativeActivityChild{}
	for _, child := range form.Children {
		if child.ID == "" || incoming[child.ID].ID != "" {
			return constUtil.ErrAdminInput
		}
		incoming[child.ID] = child
	}
	children := []p15SeasonalChild{}
	for index, current := range snap.children {
		child, ok := incoming[current.ID]
		if !ok || child.Domain != current.Domain || child.Domain != "period" && child.Domain != "commerce" {
			return constUtil.ErrAdminInput
		}
		base := snap.childForms[index]
		if base.Mode == "" || base.Mode == "unsupported" {
			return constUtil.ErrAdminInput
		}
		definition, ok := adminActivityDefinitionFor(base.Mode)
		if !ok || definition.domain != child.Domain {
			return constUtil.ErrAdminInput
		}
		updated := child.Form
		if updated.Mode != "" && updated.Mode != base.Mode || updated.TemplateID != "" && updated.TemplateID != base.TemplateID {
			return constUtil.ErrAdminInput
		}
		updated.Mode = base.Mode
		updated.TemplateID = base.TemplateID
		updated.Name = adminNativeChildLabel(child.Domain)
		updated.Source = base.Source
		updated.Alias = base.Alias
		updated.RepeatPolicy = base.RepeatPolicy
		updated.StartAt = form.StartAt
		updated.EndAt = form.EndAt
		updated.ClaimEndAt = form.ClaimEndAt
		updated.Options = base.Options
		if child.Domain == "period" {
			updated.Products = base.Products
			updated.Prizes = base.Prizes
		} else {
			updated.Milestones = base.Milestones
			updated.Prizes = base.Prizes
		}
		if err := s.adminPersistTypedActivity(tx, definition, current.ID, updated, req.Enabled, true); err != nil {
			return err
		}
		children = append(children, p15SeasonalChild{ID: current.ID, Title: adminNativeChildLabel(child.Domain), Domain: child.Domain})
	}
	updated := snap.bundle
	updated.StartAt = form.StartAt
	updated.EndAt = form.EndAt
	updated.RewardEndAt = form.ClaimEndAt
	updated.Children = adminJSON(children)
	updated.Enabled = req.Enabled
	if _, _, err := p15SeasonalSnapshot(updated); err != nil {
		return constUtil.ErrAdminInput
	}
	if err := tx.Model(&db.P15SeasonalBundle{}).Where("id = ?", updated.ID).Updates(map[string]any{
		"start_at":      updated.StartAt,
		"end_at":        updated.EndAt,
		"reward_end_at": updated.RewardEndAt,
		"children":      updated.Children,
		"enabled":       updated.Enabled,
	}).Error; err != nil {
		return err
	}
	if req.Enabled {
		now := time.UnixMilli(updated.StartAt)
		for _, child := range children {
			visible, err := s.p15SeasonalChildVisible(tx, updated, child, now)
			if err != nil || !visible {
				return constUtil.ErrAdminInput
			}
		}
	}
	return nil
}

func (s *GameService) adminNativeApplyFrozenState(tx *gorm.DB, snap *adminNativeSnapshot, enabled bool) error {
	if enabled {
		if err := s.adminNativeValidateEnable(tx, snap); err != nil {
			return err
		}
	}
	switch snap.dto.Kind {
	case "fund":
		return tx.Model(&db.ActivityRules{}).Where("id = ?", snap.fund.ID).Update("enabled", enabled).Error
	case "payment-first", "payment-single", "payment-total", "payment-continued", "payment-consume":
		return tx.Model(&db.PaymentActivityRule{}).Where("id = ?", snap.payment.ID).Update("enabled", enabled).Error
	case "month-card", "permanent-card":
		return tx.Model(&db.PaymentProduct{}).Where("id = ?", snap.product.ID).Update("enabled", enabled).Error
	case "vip-benefits":
		return tx.Model(&db.AdminNativeActivityPolicy{}).Where("id = ?", "vip-benefits").Update("enabled", enabled).Error
	case "copy-benefits":
		return tx.Model(&db.P4CopyBenefitRule{}).Where("id = ?", snap.copyRule.ID).Update("enabled", enabled).Error
	case "recruit-rotation":
		return tx.Model(&db.P4RecruitRotation{}).Where("id = ?", snap.rotation.ID).Update("enabled", enabled).Error
	case "first-double":
		return tx.Model(&db.FirstDoublePolicy{}).Where("id = ?", 1).Update("enabled", enabled).Error
	case "celebration":
		return tx.Model(&db.PeriodInstance{}).Where("id = ?", snap.period.ID).Update("enabled", enabled).Error
	case "festival", "update":
		if err := tx.Model(&db.P15SeasonalBundle{}).Where("id = ?", snap.bundle.ID).Update("enabled", enabled).Error; err != nil {
			return err
		}
		for _, child := range snap.children {
			table := ""
			if child.Domain == "period" {
				table = "p1_period_instances"
			} else {
				table = "p4_commerce_rules"
			}
			if err := tx.Table(table).Where("id = ?", child.ID).Update("enabled", enabled).Error; err != nil {
				return err
			}
		}
		if enabled {
			effective, err := adminNativeP15Effective(snap.bundle)
			if err != nil {
				return err
			}
			now := time.UnixMilli(effective.StartAt)
			for _, child := range snap.children {
				visible, err := s.p15SeasonalChildVisible(tx, effective, child, now)
				if err != nil || !visible {
					return constUtil.ErrAdminInput
				}
			}
		}
		return nil
	}
	return constUtil.ErrAdminInput
}

// adminNativeValidateEnable 对未来开放做真实消费者校验，空奖励或残缺目录不会因开关被放行。
func (s *GameService) adminNativeValidateEnable(tx *gorm.DB, snap *adminNativeSnapshot) error {
	switch snap.dto.Kind {
	case "fund":
		if snap.fund.MinVIP < 0 || snap.fund.DiamondCost <= 0 || snap.fund.StartAt >= snap.fund.EndAt || snap.fund.EndAt > snap.fund.RewardEndAt || len(snap.fundRewards) == 0 {
			return constUtil.ErrAdminInput
		}
		for _, row := range snap.fundRewards {
			ok, err := s.openServerRewardSupported(row.Reward)
			if err != nil || !ok || row.Threshold <= 0 {
				return constUtil.ErrAdminInput
			}
		}
	case "payment-first", "payment-single", "payment-total", "payment-continued", "payment-consume":
		if snap.payment.StartAt >= snap.payment.EndAt || snap.payment.EndAt > snap.payment.RewardEndAt || snap.payment.Threshold <= 0 || snap.payment.Reward == "" {
			return constUtil.ErrAdminInput
		}
		if _, err := p3Resources(snap.payment.Reward); err != nil {
			return constUtil.ErrAdminInput
		}
	case "month-card", "permanent-card":
		product := snap.product
		if product.Free <= 0 {
			return constUtil.ErrAdminInput
		}
		if err := validateP3Product(&product); err != nil {
			return constUtil.ErrAdminInput
		}
	case "copy-benefits":
		if _, err := adminNativeCopyEffective(snap.copyRule); err != nil {
			return err
		}
	case "recruit-rotation":
		if _, _, err := s.p4RotationSnapshot(snap.rotation, 2); err != nil {
			return constUtil.ErrAdminInput
		}
	case "first-double":
		if snap.firstDouble.Version != constUtil.FirstDoubleVersion || snap.firstDouble.Source != constUtil.FirstDoubleSource {
			return constUtil.ErrAdminInput
		}
	case "celebration":
		effective, err := adminNativePeriodEffective(snap.period)
		if err != nil {
			return err
		}
		if err := s.validateP1Period(effective); err != nil {
			return constUtil.ErrAdminInput
		}
	case "festival", "update":
		effective, err := adminNativeP15Effective(snap.bundle)
		if err != nil {
			return err
		}
		var children []p15SeasonalChild
		if len(effective.Children) > 16384 || json.Unmarshal(effective.Children, &children) != nil {
			return constUtil.ErrAdminInput
		}
		effective.Enabled = true
		if _, _, err := p15SeasonalSnapshot(effective); err != nil {
			return constUtil.ErrAdminInput
		}
		now := time.UnixMilli(effective.StartAt)
		for _, child := range children {
			if child.Domain == "period" {
				var row db.PeriodInstance
				if err := tx.First(&row, "id = ?", child.ID).Error; err != nil {
					return constUtil.ErrAdminInput
				}
				row, err = adminNativePeriodEffective(row)
				if err != nil {
					return constUtil.ErrAdminInput
				}
				row.Enabled = true
				if err := s.validateP1Period(row); err != nil || row.StartAt != effective.StartAt || row.EndAt != effective.EndAt || row.RewardEndAt != effective.RewardEndAt {
					return constUtil.ErrAdminInput
				}
			} else {
				var row db.P4CommerceRule
				if err := tx.First(&row, "id = ?", child.ID).Error; err != nil {
					return constUtil.ErrAdminInput
				}
				frozen, _, err := s.p4CommerceSnapshot(row)
				if err != nil || frozen.StartAt != effective.StartAt || frozen.EndAt != effective.EndAt || frozen.ClaimEndAt > effective.RewardEndAt {
					return constUtil.ErrAdminInput
				}
			}
			if now.UnixMilli() < effective.StartAt || now.UnixMilli() >= effective.RewardEndAt {
				return constUtil.ErrAdminInput
			}
		}
	case "vip-benefits":
		return nil
	}
	return nil
}

// adminNativeDefaultVIPForm 是缺行时消费者和后台共同使用的当前硬编码策略。
func adminNativeDefaultVIPForm() model.AdminNativeActivityForm {
	form := model.AdminNativeActivityForm{VipTiers: []model.AdminNativeVIPTier{}}
	for vip := int64(0); vip <= 12; vip++ {
		form.VipTiers = append(form.VipTiers, model.AdminNativeVIPTier{
			Vip:          vip,
			DailyRewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: (vip + 1) * 1000}},
			WeekCost:     model.AdminResourceAmount{ResourceID: "diamond", Amount: 100},
			WeekRewards:  []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 10}},
		})
	}
	return form
}

// adminNativeVIPPolicy 读取已保存会员福利；缺行或未建表时继续使用当前默认策略。
func (s *GameService) adminNativeVIPPolicy(tx *gorm.DB, lock bool) (model.AdminNativeActivityForm, bool, bool, error) {
	if !tx.Migrator().HasTable(&db.AdminNativeActivityPolicy{}) {
		return adminNativeDefaultVIPForm(), true, false, nil
	}
	var policy db.AdminNativeActivityPolicy
	query := tx.Where("id = ?", "vip-benefits")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminNativeDefaultVIPForm(), true, false, nil
	}
	if err != nil {
		return model.AdminNativeActivityForm{}, false, false, err
	}
	var form model.AdminNativeActivityForm
	if err := json.Unmarshal(policy.Form, &form); err != nil {
		return model.AdminNativeActivityForm{}, false, false, constUtil.ErrAdminInput
	}
	form, err = s.adminNativeValidateVIPTiers(form, policy.Enabled)
	if err != nil {
		return model.AdminNativeActivityForm{}, false, false, err
	}
	return form, policy.Enabled, true, nil
}

func (s *GameService) adminNativeValidateVIPTiers(form model.AdminNativeActivityForm, enabled bool) (model.AdminNativeActivityForm, error) {
	if len(form.VipTiers) != 13 {
		return model.AdminNativeActivityForm{}, constUtil.ErrAdminInput
	}
	tiers := make([]model.AdminNativeVIPTier, 13)
	seen := map[int64]bool{}
	for _, tier := range form.VipTiers {
		if tier.Vip < 0 || tier.Vip > 12 || seen[tier.Vip] {
			return model.AdminNativeActivityForm{}, constUtil.ErrAdminInput
		}
		seen[tier.Vip] = true
		tiers[tier.Vip] = tier
	}
	for vip := int64(0); vip <= 12; vip++ {
		if !seen[vip] {
			return model.AdminNativeActivityForm{}, constUtil.ErrAdminInput
		}
		tier := &tiers[vip]
		if _, err := s.adminTypedResourceSpec(tier.DailyRewards, !enabled); err != nil {
			return model.AdminNativeActivityForm{}, err
		}
		if enabled && len(tier.DailyRewards) == 0 {
			return model.AdminNativeActivityForm{}, constUtil.ErrAdminInput
		}
		if tier.WeekCost.ResourceID != "diamond" || tier.WeekCost.Amount < 1 || tier.WeekCost.Amount > math.MaxInt32 {
			return model.AdminNativeActivityForm{}, constUtil.ErrAdminInput
		}
		if _, err := s.adminTypedResourceSpec(tier.WeekRewards, !enabled); err != nil {
			return model.AdminNativeActivityForm{}, err
		}
		if enabled && len(tier.WeekRewards) == 0 {
			return model.AdminNativeActivityForm{}, constUtil.ErrAdminInput
		}
	}
	form.VipTiers = tiers
	return adminNativeNormalizeForm(form), nil
}

func adminNativeVIPTierFor(form model.AdminNativeActivityForm, vip int64) (model.AdminNativeVIPTier, bool) {
	for _, tier := range form.VipTiers {
		if tier.Vip == vip {
			return tier, true
		}
	}
	if len(form.VipTiers) > 0 {
		return form.VipTiers[len(form.VipTiers)-1], true
	}
	return model.AdminNativeVIPTier{}, false
}

// adminNativePBResources 将已校验配置投影为原协议资源；损坏行在发奖前阻断。
func adminNativePBResources(values []model.AdminResourceAmount) ([]*pb.BaseResource, error) {
	out := []*pb.BaseResource{}
	seen := map[string]bool{}
	for _, value := range values {
		if value.ResourceID == "" || strings.ContainsAny(value.ResourceID, ",; \r\n") || value.Amount < 1 || value.Amount > math.MaxInt32 || seen[value.ResourceID] {
			return nil, constUtil.ErrAdminInput
		}
		seen[value.ResourceID] = true
		out = append(out, &pb.BaseResource{Id: proto.String(value.ResourceID), Num: proto.Int32(int32(value.Amount))})
	}
	return out, nil
}

// adminNativePBResourceSpec 还原领取与审计使用的固定奖励串，顺序与前台展示一致。
func adminNativePBResourceSpec(values []*pb.BaseResource) (string, error) {
	if len(values) == 0 || len(values) > 100 {
		return "", constUtil.ErrAdminInput
	}
	seen := map[string]bool{}
	parts := []string{}
	for _, value := range values {
		if value == nil || value.GetId() == "" || strings.ContainsAny(value.GetId(), ",; \r\n") || value.GetNum() < 1 || value.GetGuid() != "" || seen[value.GetId()] {
			return "", constUtil.ErrAdminInput
		}
		seen[value.GetId()] = true
		parts = append(parts, value.GetId()+","+strconv.Itoa(int(value.GetNum())))
	}
	return strings.Join(parts, ";"), nil
}

// AdminRemoveNativeActivity 从运营列表移除停用配置，保留源配置、回执与玩家历史。
func (s *GameService) AdminRemoveNativeActivity(ctx context.Context, p *model.AdminPrincipal, id, version string) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		snap, err := s.adminNativeLoad(tx, id, true)
		if err != nil {
			return err
		}
		meta, err := adminNativeMeta(tx, id, true)
		if err != nil {
			return err
		}
		if meta.Archived || snap.dto.Version != version || snap.dto.Enabled {
			return constUtil.ErrAdminConfigConflict
		}
		meta.ActivityID, meta.Mode, meta.TemplateID = id, "native-"+snap.dto.Kind, "native-"+snap.dto.Kind
		meta.Name, meta.Alias, meta.Archived, meta.UpdatedAtMillis = snap.dto.Name, snap.dto.Alias, true, time.Now().UnixMilli()
		if err = tx.Save(&meta).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "native-activity.remove", id, "移除停用配置；保留源数据及玩家历史")
	})
}

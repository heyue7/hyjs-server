package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// Every private domain row retains its native routing prefix and carries one
// immutable cycle identifier. Historical rows are never reset or reassigned.
var localPlayCyclePattern = regexp.MustCompile(`lp-[0-9a-f]{26}`)
var errLocalPlayActivitySource = errors.New("原活动有效配置尚未就绪")

type localPlayScopedSnapshot struct {
	Periods   []db.PeriodInstance             `json:"periods,omitempty"`
	Welfare   []db.P4WelfareRule              `json:"welfare,omitempty"`
	Commerce  []db.P4CommerceRule             `json:"commerce,omitempty"`
	Payments  []db.PaymentActivityRule        `json:"payments,omitempty"`
	Bundles   []db.P15SeasonalBundle          `json:"bundles,omitempty"`
	Copy      []db.P4CopyBenefitRule          `json:"copy,omitempty"`
	Rotations []db.P4RecruitRotation          `json:"rotations,omitempty"`
	MonthSign []db.MonthSignRule              `json:"monthSign,omitempty"`
	Exchange  *db.P4ResourceExchangeDirectory `json:"exchange,omitempty"`
	EntryOnly bool                            `json:"entryOnly,omitempty"`
}

func localPlayPrivateCycleID(key string) string { return localPlayCyclePattern.FindString(key) }

// Workers have no player session. They use the immutable cycle owner and that
// owner's current calendar offset, even for retired cycles and closed tooling.
// This does not authorize business requests or unlock a game activity.
func localPlayCycleNow(tx *gorm.DB, key string, real time.Time) (time.Time, error) {
	cycleID := localPlayPrivateCycleID(key)
	if cycleID == "" {
		if at, ok := ServerGameClockAt(tx.Statement.Context, real); ok {
			return at, nil
		}
		return real, nil
	}
	var cycle db.LocalPlayActivityCycle
	if err := tx.First(&cycle, "id=?", cycleID).Error; err != nil {
		return real, err
	}
	if cycle.PlayerID <= 1 || cycle.Source != constUtil.LocalPlayActivitiesVersion {
		return real, errLocalPlayActivitiesInvalid
	}
	var clock db.LocalPlayClockState
	err := tx.First(&clock, "player_id=?", cycle.PlayerID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return real, err
	}
	if clock.OffsetDays < -3650 || clock.OffsetDays > 3650 || clock.Revision < 0 {
		return real, errLocalPlayActivitiesInvalid
	}
	return real.AddDate(0, 0, int(clock.OffsetDays)), nil
}

// Direct claims may read an owned retired cycle; list/fact paths request only
// the current binding. Foreign private IDs are rejected even when guessed.
func localPlayActivityAccess(tx *gorm.DB, key string, currentOnly bool) (bool, error) {
	if allowed, err := adminActivityScheduleAccess(tx, key); err != nil || !allowed {
		return allowed, err
	}
	cycleID := localPlayPrivateCycleID(key)
	if cycleID == "" {
		return true, nil
	}
	owner := localPlayActivityOwner(tx)
	if owner <= 1 {
		return false, nil
	}
	var cycle db.LocalPlayActivityCycle
	if err := tx.First(&cycle, "id=?", cycleID).Error; err != nil {
		return false, err
	}
	if cycle.PlayerID != owner || cycle.Source != constUtil.LocalPlayActivitiesVersion {
		return false, nil
	}
	if !currentOnly {
		return true, nil
	}
	var binding db.LocalPlayActivityBinding
	if err := tx.First(&binding, "player_id=? AND activity_id=?", owner, cycle.ActivityID).Error; err != nil {
		return false, err
	}
	return binding.CycleID == cycleID, nil
}

func localPlayActivityIDForConfig(key string) string {
	first := strings.SplitN(key, "_", 2)[0]
	switch first {
	case "kaiFuJingSai":
		return "OpenServerRace"
	case "pvpWinTimes":
		return "BattleRace"
	case "robTreasureTimes":
		return "RobTrea"
	case "aincradStars":
		return "AincradStars"
	case "mainCopyWinTimes":
		return "MainCopy"
	case "warriorRescruitTimes":
		return "WarriorRescruitTimes"
	case "warriorGetSpecial":
		return "CollectionWarrior"
	case "kaiGongActivity":
		return "KaiGongActivity"
	case "totalLogin", "totalLoginFestival":
		if first == "totalLoginFestival" {
			return "Festival"
		}
		return "LoginAward"
	case "totalLoginUpdate":
		return "Update"
	case "celebrationActivity":
		return "Congratulation"
	case "festival":
		return "Festival"
	case "update":
		return "Update"
	case "discountSale":
		return "DiscountSale"
	case "halfDiscountSale":
		return "HalfDiscountSale"
	case "limitExchange":
		return "TimeLimitExchange"
	case "limitExchangeFestival":
		return "Festival"
	case "limitSale":
		return "TimeLimitSail"
	case "mysticDiscount":
		return "MysticDiscount"
	case "rechargeSingle":
		return "RechargeSurprise"
	case "costTotal":
		return "CostTotal"
	case "rechargeTotal":
		return "RechargeTotal"
	case "continuedRecharge":
		return "continueRechargeGift"
	case "allCopyDouble":
		return "CopyBenefits"
	}
	if strings.HasPrefix(key, "p4:copy:") {
		return "CopyBenefits"
	}
	if strings.HasPrefix(key, "p4:recruit:") {
		return "WarriorWeightImprove"
	}
	if strings.HasPrefix(key, "p4:") {
		parts := strings.Split(key, ":")
		if len(parts) > 1 {
			return localPlayWelfareActivity(parts[1])
		}
	}
	return ""
}

// Suppress the legacy global counterpart only for an authenticated owner who
// explicitly opened this activity. Other accounts keep the original catalogue.
func localPlayActivityVisible(tx *gorm.DB, key string) (bool, error) {
	if allowed, err := adminActivityScheduleAccess(tx, key); err != nil || !allowed {
		return allowed, err
	}
	if localPlayPrivateCycleID(key) != "" {
		return localPlayActivityAccess(tx, key, true)
	}
	// 已退休试玩绑定仅在其原个人时钟上下文选择旧实例。
	// 普通全服时钟请求继续服从后台日历，不被旧绑定隐藏。
	if !LocalPlayContextBound(tx.Statement.Context) {
		return true, nil
	}
	owner := localPlayActivityOwner(tx)
	activity := localPlayActivityIDForConfig(key)
	if owner <= 1 || activity == "" || !gameSchemaHasTable(tx, &db.LocalPlayActivityBinding{}) {
		return true, nil
	}
	var count int64
	err := tx.Model(&db.LocalPlayActivityBinding{}).Where("player_id=? AND activity_id=?", owner, activity).Count(&count).Error
	return count == 0, err
}

func localPlayWelfareActivity(kind string) string {
	switch kind {
	case constUtil.P4WelfareLogin:
		return "LoginAward"
	case constUtil.P4WelfareLimit:
		return "EveryDaySign"
	case constUtil.P4WelfareMeal:
		return "AsunaKitchen"
	case constUtil.P4WelfareCat:
		return "fortuneCat"
	case constUtil.P4WelfareRebate:
		return "CostReturn"
	}
	return ""
}

func localPlayWelfareRuleID(tx *gorm.DB, kind string) (string, error) {
	cycle, err := localPlayBoundCycle(tx, localPlayActivityOwner(tx), localPlayWelfareActivity(kind))
	if err != nil || cycle == nil {
		return "", err
	}
	var snapshot localPlayScopedSnapshot
	if json.Unmarshal(cycle.Snapshot, &snapshot) != nil {
		return "", errLocalPlayActivitiesInvalid
	}
	for _, rule := range snapshot.Welfare {
		if rule.Kind == kind {
			return rule.ID, nil
		}
	}
	return "", errLocalPlayActivitiesInvalid
}

func localPlayEntryCycle(tx *gorm.DB, activity string) (*db.LocalPlayActivityCycle, error) {
	return localPlayBoundCycle(tx, localPlayActivityOwner(tx), activity)
}

func localPlayExchangeKey(tx *gorm.DB, now time.Time) (string, error) {
	cycle, err := localPlayEntryCycle(tx, "TimeLimitExchange")
	if err != nil || cycle == nil {
		return p4ExchangeKey(now), err
	}
	var snapshot localPlayScopedSnapshot
	if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || snapshot.Exchange == nil {
		return "", errLocalPlayActivitiesInvalid
	}
	return snapshot.Exchange.ID, nil
}

func localPlayWelfareActivityKey(tx *gorm.DB, kind string, legacy string) (string, error) {
	cycle, err := localPlayEntryCycle(tx, localPlayWelfareActivity(kind))
	if err != nil || cycle == nil {
		return legacy, err
	}
	if kind == constUtil.P4WelfareRebate {
		return "consumeRet_p4_" + cycle.ID, nil
	}
	if kind == constUtil.P4WelfareCat {
		return "fortuneCat_p4_" + cycle.ID, nil
	}
	return legacy, nil
}

// Source availability follows enabled operator templates; private historical
// cycles cannot make a disabled offering available for another refresh.
func localPlayActivitySourceReady(tx *gorm.DB, def localPlayActivityDefinition) bool {
	exists := func(table interface{}, condition string, args ...interface{}) bool {
		if !gameSchemaHasTable(tx, table) {
			return false
		}
		var count int64
		query := tx.Model(table).Where("enabled=? AND id NOT LIKE ?", true, "%lp-%")
		return query.Where(condition, args...).Count(&count).Error == nil && count > 0
	}
	periodReady := func(prefix string) bool {
		return prefix != "" && exists(&db.PeriodInstance{}, "id LIKE ?", prefix+"_%")
	}
	welfareKind := map[string]string{"EveryDaySign": constUtil.P4WelfareLimit, "LoginAward": constUtil.P4WelfareLogin, "AsunaKitchen": constUtil.P4WelfareMeal, "fortuneCat": constUtil.P4WelfareCat, "CostReturn": constUtil.P4WelfareRebate}[def.ID]
	switch def.Kind {
	case "entry":
		return true
	case "open-server":
		return gameSchemaHasTable(tx, &db.OpenServerActivityTask{})
	case "turntable":
		return gameSchemaHasTable(tx, &db.GameplayPool{})
	case "period":
		prefix := map[string]string{"OpenServerRace": "kaiFuJingSai", "MainCopy": "mainCopyWinTimes", "AincradStars": "aincradStars", "WarriorRescruitTimes": "warriorRescruitTimes", "CollectionWarrior": "warriorGetSpecial", "KaiGongActivity": "kaiGongActivity", "Congratulation": "celebrationActivity"}[def.ID]
		return periodReady(prefix)
	case "login", "welfare":
		if welfareKind == "" || !exists(&db.P4WelfareRule{}, "kind=?", welfareKind) {
			return false
		}
		if def.ID == "LoginAward" {
			return periodReady("totalLogin_p1")
		}
		if def.ID == "EveryDaySign" {
			if !gameSchemaHasTable(tx, &db.MonthSignRule{}) {
				return false
			}
			var count int64
			return tx.Model(&db.MonthSignRule{}).Where("enabled=? AND day>=? AND day<=?", true, 1, 31).Count(&count).Error == nil && count == 31
		}
		return true
	case "commerce":
		kind := map[string]string{"DiscountSale": "discount", "HalfDiscountSale": "half", "TimeLimitExchange": "exchange", "TimeLimitSail": "sale", "MysticDiscount": "mystic"}[def.ID]
		return kind != "" && exists(&db.P4CommerceRule{}, "kind=? AND shop_type=0 AND id NOT LIKE ? AND id NOT LIKE ?", kind, "limitExchangeFestival_%", constUtil.P15RedOSSupplyPrefix+"%")
	case "payment":
		kind := map[string]string{"RechargeSurprise": "single", "CostTotal": "consume", "RechargeTotal": "total", "continueRechargeGift": "continued"}[def.ID]
		return kind != "" && exists(&db.PaymentActivityRule{}, "kind=?", kind)
	case "seasonal":
		kind := strings.ToLower(def.ID)
		prefix := map[string]string{"Festival": "totalLoginFestival", "Update": "totalLoginUpdate"}[def.ID]
		if !periodReady(prefix) || !exists(&db.P15SeasonalBundle{}, "kind=?", kind) {
			return false
		}
		return def.ID != "Festival" || exists(&db.P4CommerceRule{}, "kind=? AND shop_type=0 AND id LIKE ?", "exchange", "limitExchangeFestival_%")
	case "copy":
		return exists(&db.P4CopyBenefitRule{}, "1=1")
	case "rotation":
		return exists(&db.P4RecruitRotation{}, "kind=?", 1) && exists(&db.P4RecruitRotation{}, "kind=?", 2)
	default:
		return false
	}
}

// No old payment or event qualifies a new period merely because it happened
// earlier on the same calendar day. Call before querying original metrics.
func localPlayActivityMetricStart(tx *gorm.DB, key string, start int64) (int64, error) {
	cycleID := localPlayPrivateCycleID(key)
	if cycleID == "" {
		return start, nil
	}
	allowed, err := localPlayActivityAccess(tx, key, false)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, errLocalPlayActivitiesInvalid
	}
	var cycle db.LocalPlayActivityCycle
	if err := tx.First(&cycle, "id=?", cycleID).Error; err != nil {
		return 0, err
	}
	return max(start, cycle.CreatedAt), nil
}

func localPlayCloneKey(key, cycleID string, ordinal int) string {
	if strings.HasSuffix(key, "_local_p3") {
		return strings.SplitN(key, "_", 2)[0] + "_" + cycleID + "_local_p3"
	}
	if strings.HasPrefix(key, "p4:") {
		parts := strings.Split(key, ":")
		return parts[0] + ":" + parts[1] + ":" + cycleID
	}
	if strings.HasPrefix(key, "p4-copy-") {
		return "allCopyDouble_p4_" + cycleID
	}
	if isP15SeasonalChild(key) {
		parts := strings.Split(key, "_")
		return parts[0] + "_p4_" + parts[2] + "_" + cycleID
	}
	if strings.HasPrefix(key, "totalLogin_p1_") {
		if strings.HasSuffix(key, "_three") {
			return "totalLogin_p1_" + cycleID + "_three"
		}
		return "totalLogin_p1_" + cycleID + "_welcome"
	}
	if at := strings.LastIndex(key, "_"); at >= 0 {
		return key[:at+1] + cycleID
	}
	return strings.SplitN(key, "_", 2)[0] + "_p4_" + cycleID
}

func localPlayDomainContext(tx *gorm.DB) *gorm.DB {
	// Source selection never selects an earlier private clone.
	return tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.LocalPlayOwnerKey, int64(0)))
}

func (s *GameService) localPlayCreateScopedCycle(tx *gorm.DB, id int64, def localPlayActivityDefinition, cycle *db.LocalPlayActivityCycle, now time.Time) (*db.LocalPlayActivityCycle, error) {
	snapshot := localPlayScopedSnapshot{}
	cycle.StartAt = now.UnixMilli()
	cycle.EndAt = now.AddDate(0, 1, 0).UnixMilli()
	cycle.RewardEndAt = now.AddDate(0, 1, 7).UnixMilli()
	source := localPlayDomainContext(tx)
	switch def.Kind {
	case "entry":
		snapshot.EntryOnly = true
	case "period", "login", "seasonal":
		prefixes := map[string][]string{
			"OpenServerRace": {"kaiFuJingSai"}, "BattleRace": {"pvpWinTimes"}, "RobTrea": {"robTreasureTimes"}, "MainCopy": {"mainCopyWinTimes"},
			"AincradStars":         {"aincradStars"},
			"WarriorRescruitTimes": {"warriorRescruitTimes"}, "CollectionWarrior": {"warriorGetSpecial"}, "KaiGongActivity": {"kaiGongActivity"},
			"LoginAward": {"totalLogin"}, "Congratulation": {"celebrationActivity"}, "Festival": {"totalLoginFestival"}, "Update": {"totalLoginUpdate"},
		}[def.ID]
		var originals []db.PeriodInstance
		if err := source.Where("enabled=? AND id NOT LIKE ?", true, "%lp-%").Order("start_at DESC,id DESC").Find(&originals).Error; err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, live := range originals {
			prefix := strings.SplitN(live.ID, "_", 2)[0]
			wanted := false
			for _, p := range prefixes {
				wanted = wanted || prefix == p
			}
			if !wanted {
				continue
			}
			identity := prefix
			if prefix == "totalLogin" {
				if strings.HasSuffix(live.ID, "_three") {
					identity += "three"
				} else {
					identity += "welcome"
				}
			}
			if seen[identity] {
				continue
			}
			seen[identity] = true
			cfg, err := s.freezeP1Period(source, live, false)
			if err != nil {
				return nil, err
			}
			// The personal trial period counts only newly accepted earned stars.
			// The global peak fact retains its existing original consumer.
			if def.ID == "AincradStars" {
				if cfg.Kind != constUtil.P7RushStarEvent || cfg.Mode != "single" || cfg.Goal != 3 || cfg.Rewards != "point,100" {
					return nil, errLocalPlayActivitySource
				}
				cfg.Kind, cfg.Mode = constUtil.P7RushEarnedStarEvent, "cumulative"
				cfg.Source += constUtil.P7AincradPersonalSource
			}
			cfg.ID = localPlayCloneKey(cfg.ID, cycle.ID, len(snapshot.Periods))
			cfg.Frozen = nil
			cfg.StartAt, cfg.EndAt, cfg.RewardEndAt = cycle.StartAt, cycle.EndAt, cycle.RewardEndAt
			if err := s.validateP1Period(cfg); err != nil {
				return nil, err
			}
			if err := tx.Create(&cfg).Error; err != nil {
				return nil, err
			}
			snapshot.Periods = append(snapshot.Periods, cfg)
		}
		if len(snapshot.Periods) == 0 {
			return nil, errLocalPlayActivitySource
		}
	}
	if def.Kind == "welfare" || def.Kind == "login" {
		kind := map[string]string{"EveryDaySign": constUtil.P4WelfareLimit, "LoginAward": constUtil.P4WelfareLogin, "AsunaKitchen": constUtil.P4WelfareMeal, "fortuneCat": constUtil.P4WelfareCat, "CostReturn": constUtil.P4WelfareRebate}[def.ID]
		var cfg db.P4WelfareRule
		if err := source.Where("kind=? AND enabled=? AND id NOT LIKE ?", kind, true, "%lp-%").Order("start_at DESC,id DESC").First(&cfg).Error; err != nil {
			return nil, err
		}
		if len(cfg.Frozen) > 0 {
			if err := json.Unmarshal(cfg.Frozen, &cfg); err != nil {
				return nil, err
			}
		}
		cfg.ID = localPlayCloneKey(cfg.ID, cycle.ID, 0)
		cfg.Frozen = nil
		cfg.Enabled = true
		cfg.StartAt, cfg.EndAt, cfg.ClaimEndAt = cycle.StartAt, cycle.EndAt, cycle.RewardEndAt
		if kind == constUtil.P4WelfareLimit || kind == constUtil.P4WelfareMeal || kind == constUtil.P4WelfareRebate {
			cfg.ClaimEndAt = cfg.EndAt
			cycle.RewardEndAt = cycle.EndAt
		}
		if _, err := s.validateP4Welfare(cfg); err != nil {
			return nil, err
		}
		if err := tx.Create(&cfg).Error; err != nil {
			return nil, err
		}
		snapshot.Welfare = append(snapshot.Welfare, cfg)
		if def.ID == "EveryDaySign" {
			if err := source.Order("day ASC").Find(&snapshot.MonthSign).Error; err != nil {
				return nil, err
			}
			if len(snapshot.MonthSign) != 31 {
				return nil, errLocalPlayActivitySource
			}
			for i, rule := range snapshot.MonthSign {
				if rule.Day != int32(i+1) || !rule.Enabled || rule.DoubleVIP < 0 {
					return nil, errLocalPlayActivitiesInvalid
				}
				if _, err := activityLoginResources(rule.Reward); err != nil {
					return nil, err
				}
			}
		}
	}
	if def.Kind == "commerce" || def.ID == "Festival" {
		kind := map[string]string{"DiscountSale": "discount", "HalfDiscountSale": "half", "TimeLimitExchange": "exchange", "TimeLimitSail": "sale", "MysticDiscount": "mystic", "Festival": "exchange"}[def.ID]
		var originals []db.P4CommerceRule
		if err := source.Where("kind=? AND shop_type=0 AND enabled=? AND id NOT LIKE ?", kind, true, "%lp-%").Order("start_at DESC,id DESC").Find(&originals).Error; err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, live := range originals {
			identity := live.ID
			if at := strings.LastIndex(identity, "_"); at >= 0 {
				identity = identity[:at]
			}
			// The named OS supply retains monthly quotas across private refreshes.
			if strings.HasPrefix(live.ID, constUtil.P15RedOSSupplyPrefix) {
				continue
			}
			if (def.ID == "Festival") != isP15SeasonalChild(live.ID) || seen[identity] {
				continue
			}
			seen[identity] = true
			cfg, _, err := s.p4CommerceSnapshot(live)
			if err != nil {
				return nil, err
			}
			cfg.ID = localPlayCloneKey(cfg.ID, cycle.ID, len(snapshot.Commerce))
			cfg.Frozen = nil
			cfg.StartAt, cfg.EndAt, cfg.ClaimEndAt = cycle.StartAt, cycle.EndAt, cycle.EndAt
			if _, err := s.validateP4Commerce(cfg); err != nil {
				return nil, err
			}
			if err := tx.Create(&cfg).Error; err != nil {
				return nil, err
			}
			snapshot.Commerce = append(snapshot.Commerce, cfg)
		}
		if len(snapshot.Commerce) == 0 {
			return nil, errLocalPlayActivitySource
		}
		if def.ID == "TimeLimitExchange" {
			bindings := []p4ExchangeBinding{}
			for _, cfg := range snapshot.Commerce {
				_, rules, err := s.p4CommerceSnapshot(cfg)
				if err != nil {
					return nil, err
				}
				for _, product := range rules.Products {
					bindings = append(bindings, p4ExchangeBinding{Index: int32(len(bindings) + 1), ChildID: cfg.ID, ChildIndex: product.Index, ProductID: product.ID})
				}
			}
			// Legacy red-currency goods retain their original independent quota and
			// stock. Refreshing this directory never resets those global goods.
			if gameSchemaHasTable(tx, &db.RedP2Exchange{}) {
				var red []db.RedP2Exchange
				if err := tx.Order("id").Find(&red).Error; err != nil {
					return nil, err
				}
				for _, product := range red {
					bindings = append(bindings, p4ExchangeBinding{Domain: "red-p2", Index: int32(len(bindings) + 1), ChildID: constUtil.P2RedExchangeActivity, ChildIndex: product.ID, ProductID: strconv.Itoa(int(product.ID))})
				}
			}
			raw, err := json.Marshal(bindings)
			if err != nil {
				return nil, err
			}
			digest := sha256.Sum256(raw)
			directory := db.P4ResourceExchangeDirectory{ID: constUtil.P4ResourceExchangePrefix + cycle.ID, Enabled: true, StartAt: cycle.StartAt, EndAt: cycle.EndAt, Bindings: raw, InputHash: hex.EncodeToString(digest[:]), Source: constUtil.P4ResourceExchangeSource}
			if err := tx.Create(&directory).Error; err != nil {
				return nil, err
			}
			// Bind the original monthly OS child only at new-period creation.
			// Existing private directories remain byte-identical until refreshed.
			if err := s.p15AppendRedOSBindings(tx, &directory, &bindings, now); err != nil {
				return nil, err
			}
			snapshot.Exchange = &directory
		}
	}
	if def.Kind == "payment" {
		kind := map[string]string{"RechargeSurprise": "single", "CostTotal": "consume", "RechargeTotal": "total", "continueRechargeGift": "continued"}[def.ID]
		var cfg db.PaymentActivityRule
		if err := source.Where("kind=? AND enabled=? AND id NOT LIKE ?", kind, true, "%lp-%").Order("start_at DESC,id DESC").First(&cfg).Error; err != nil {
			return nil, err
		}
		cfg.ID = localPlayCloneKey(cfg.ID, cycle.ID, 0)
		cfg.StartAt, cfg.EndAt, cfg.RewardEndAt = cycle.StartAt, cycle.EndAt, cycle.RewardEndAt
		if cfg.Threshold <= 0 || cfg.Source == "" {
			return nil, errLocalPlayActivitiesInvalid
		}
		if _, err := p3Resources(cfg.Reward); err != nil {
			return nil, err
		}
		if err := tx.Create(&cfg).Error; err != nil {
			return nil, err
		}
		snapshot.Payments = append(snapshot.Payments, cfg)
	}
	if def.Kind == "seasonal" {
		kind := strings.ToLower(def.ID)
		var cfg db.P15SeasonalBundle
		if err := source.Where("kind=? AND enabled=? AND id NOT LIKE ?", kind, true, "%lp-%").Order("start_at DESC,id DESC").First(&cfg).Error; err != nil {
			return nil, err
		}
		var e error
		cfg, _, e = p15SeasonalSnapshot(cfg)
		if e != nil {
			return nil, e
		}
		var children []p15SeasonalChild
		for _, p := range snapshot.Periods {
			children = append(children, p15SeasonalChild{ID: p.ID, Title: p.Title, Domain: "period"})
		}
		for _, p := range snapshot.Commerce {
			children = append(children, p15SeasonalChild{ID: p.ID, Title: "专精材料兑换", Domain: "commerce"})
		}
		raw, err := json.Marshal(children)
		if err != nil {
			return nil, err
		}
		cfg.ID = kind + "_p4_" + cycle.ID
		cfg.Children = raw
		cfg.Frozen = nil
		cfg.StartAt, cfg.EndAt, cfg.RewardEndAt = cycle.StartAt, cycle.EndAt, cycle.RewardEndAt
		if _, _, err := p15SeasonalSnapshot(cfg); err != nil {
			return nil, err
		}
		if err := tx.Create(&cfg).Error; err != nil {
			return nil, err
		}
		snapshot.Bundles = append(snapshot.Bundles, cfg)
	}
	if def.Kind == "copy" {
		var cfg db.P4CopyBenefitRule
		if err := source.Where("enabled=? AND id NOT LIKE ?", true, "%lp-%").Order("start_at DESC,id DESC").First(&cfg).Error; err != nil {
			return nil, err
		}
		if len(cfg.Frozen) > 0 {
			if err := json.Unmarshal(cfg.Frozen, &cfg); err != nil {
				return nil, err
			}
		}
		cfg.ID = "allCopyDouble_p4_" + cycle.ID
		cfg.Frozen = nil
		cfg.StartAt, cfg.EndAt = cycle.StartAt, cycle.EndAt
		cycle.RewardEndAt = cycle.EndAt
		if err := validateP4CopyBenefit(cfg); err != nil {
			return nil, err
		}
		if err := tx.Create(&cfg).Error; err != nil {
			return nil, err
		}
		snapshot.Copy = append(snapshot.Copy, cfg)
	}
	if def.Kind == "rotation" {
		// Reuse the original configuration generator inside a savepoint. Its
		// global rows are rolled back; only the authenticated owner's clones
		// remain in the committed transaction.
		if err := tx.SavePoint("local_play_rotation_source").Error; err != nil {
			return nil, err
		}
		if err := s.seedP4Rotations(source, now); err != nil {
			return nil, err
		}
		var templates []db.P4RecruitRotation
		for kind := int32(1); kind <= 2; kind++ {
			var cfg db.P4RecruitRotation
			if err := source.Where("kind=? AND enabled=? AND id NOT LIKE ? AND start_at<=? AND end_at>?", kind, true, "%lp-%", now.UnixMilli(), now.UnixMilli()).Order("start_at DESC,id DESC").First(&cfg).Error; err != nil {
				return nil, err
			}
			templates = append(templates, cfg)
		}
		if err := tx.RollbackTo("local_play_rotation_source").Error; err != nil {
			return nil, err
		}
		for _, original := range templates {
			cfg := original
			duration := cfg.EndAt - cfg.StartAt
			if duration <= 0 || duration > 7*24*3600*1000 {
				return nil, errLocalPlayActivitiesInvalid
			}
			if len(cfg.Frozen) > 0 {
				cfg.Rewards = append([]byte(nil), cfg.Frozen...)
			}
			cfg.Frozen = nil
			if cfg.Kind == 1 {
				cfg.ID = "normalRotation_" + cycle.ID
			} else {
				cfg.ID = "strongRotation_" + cycle.ID
			}
			cfg.StartAt, cfg.EndAt = cycle.StartAt, cycle.StartAt+duration
			if err := tx.Create(&cfg).Error; err != nil {
				return nil, err
			}
			snapshot.Rotations = append(snapshot.Rotations, cfg)
			cycle.EndAt = min(cycle.EndAt, cfg.EndAt)
		}
		cycle.RewardEndAt = cycle.EndAt
	}
	if !snapshot.EntryOnly && len(snapshot.Periods)+len(snapshot.Welfare)+len(snapshot.Commerce)+len(snapshot.Payments)+len(snapshot.Bundles)+len(snapshot.Copy)+len(snapshot.Rotations) == 0 {
		return nil, errLocalPlayActivitiesInvalid
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	cycle.Snapshot = raw
	return cycle, nil
}

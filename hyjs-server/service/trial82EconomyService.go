package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func trial82FloorPoints(floor, star int32) int32 {
	return (constUtil.Trial82FloorBasePoints + floor) * star
}

// Read the published rule so a historical frozen fight retains its own reward.
func trial82StagePoints(stage db.P7RushStageRule) int32 {
	rewards, _ := p1RewardResources(stage.Reward)
	for _, reward := range rewards {
		if reward.GetId() == "point" {
			return reward.GetNum()
		}
	}
	return 0
}

func (s *GameService) trial82ShopRules(rules p4CommerceRules) (p4CommerceRules, error) {
	weights := map[string]int64{"item_005": 25, "item_008": 20, "item_012": 20, "item_007": 25, "item_006": 25, "item_013": 15, "item_009": 20, "item_010": 20, "item_017": 6}
	sacred := map[string]bool{"frag_equip_037": true, "frag_equip_038": true, "frag_equip_039": true, "frag_equip_040": true}
	seen := map[string]bool{}
	for i := range rules.Products {
		p := &rules.Products[i]
		id, _, ok := strings.Cut(p.Rewards, ",")
		if !ok {
			return rules, constUtil.ErrAdminConfiguration
		}
		if !p.isEnabled() {
			continue
		}
		if sacred[id] {
			if seen[id] {
				return rules, fmt.Errorf("%w: 神圣碎片商品重复 %s", constUtil.ErrAdminConfigConflict, id)
			}
			seen[id] = true
			p.Rewards = id + ",5"
			p.Cost = fmt.Sprintf("point,%d", constUtil.Trial82SacredPackPoints)
			p.Priority = 1000 + p.Priority%1000
			p.Weight = 11
			p.DailyLimit, p.TotalLimit, p.AccountWeeklyLimit = 1, 0, 0
			p.DiscountBps = 10000
			p.TierID, p.QuantityTiers = false, nil
			continue
		}
		if weight, ok := weights[id]; ok {
			p.Weight = weight
			p.DailyLimit, p.TotalLimit, p.AccountWeeklyLimit = 1, 0, 0
			p.QuantityTiers = nil
		}
	}
	if len(seen) != len(sacred) {
		return rules, fmt.Errorf("%w: 缺少四部位神圣装备碎片", constUtil.ErrAdminConfigConflict)
	}
	rules.LiveEditable, rules.TrialRefreshVersion = true, 1
	rules.Slots, rules.FreeCap, rules.FreeSeconds, rules.ResourceRefreshCap = 6, 10, 7200, 15
	rules.RefreshItem, rules.RefreshCost = "item_013,1", "diamond,20"
	enabled := true
	rules.RefreshEnabled = &enabled
	return rules, nil
}

// ApplyTrial82Config runs explicitly during maintenance. It changes operating
// rules and scheduled consumers only; assets, accepted fights and receipts stay intact.
func (s *GameService) ApplyTrial82Config(ctx context.Context, actor *model.AdminPrincipal) error {
	if s.OriginalServerID() != 1 {
		return constUtil.ErrAdminConfiguration
	}
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var operator db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&operator, actor.UserID).Error; err != nil || !operator.Enabled || operator.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		var policy db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Maintenance {
			return fmt.Errorf("%w: 须在维护期执行", constUtil.ErrAdminConfigConflict)
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		// The shared synchronizer reads these bounded operating sets, never receipts.
		for _, table := range []any{&db.P4CommerceRule{}, &db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}} {
			if !gameSchemaHasTable(tx, table) {
				continue
			}
			var count int64
			if err := tx.Model(table).Count(&count).Error; err != nil {
				return err
			}
			if count > constUtil.Trial82MaxShopRows {
				return fmt.Errorf("%w: 商店维护数量超过上限", constUtil.ErrAdminConfigConflict)
			}
		}
		source, err := s.AdminOrdinaryShopSource(tx, "point")
		if err != nil {
			return err
		}
		var lockedSource db.P4CommerceRule
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedSource, "id=?", source.ID).Error; err != nil {
			return err
		}
		// Use the actual operating row, including its historical frozen evidence.
		source = lockedSource
		var rules p4CommerceRules
		if err = json.Unmarshal(source.Rules, &rules); err != nil {
			return err
		}
		rules, err = s.trial82ShopRules(rules)
		if err != nil {
			return err
		}
		next := source
		next.Rules = adminJSON(rules)
		if _, err = s.validateP4Commerce(next); err != nil {
			return err
		}
		var stages []db.P7RushStageRule
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind=? AND source=?", constUtil.P7RushTrial, constUtil.P7ReRushSource).Order("id").Limit(307).Find(&stages).Error; err != nil {
			return err
		}
		if len(stages) != 306 {
			return fmt.Errorf("%w: 试炼目录应为102层三难度", constUtil.ErrAdminConfigConflict)
		}
		seen := map[string]bool{}
		changed := false
		for _, stage := range stages {
			id := fmt.Sprintf("re-rush:%d:%d", stage.Floor, stage.Star)
			if stage.Floor < 1 || stage.Floor > 102 || stage.Star < 1 || stage.Star > 3 || stage.ID != id || seen[id] || stage.FirstReward != "" {
				return fmt.Errorf("%w: 试炼目录身份不符 %s", constUtil.ErrAdminConfigConflict, stage.ID)
			}
			seen[id] = true
			oldReward := fmt.Sprintf("coin,%d;point,%d", (2000+stage.Floor*500)*stage.Star, (50+stage.Floor*10)*stage.Star)
			newReward := fmt.Sprintf("coin,%d;point,%d", (2000+stage.Floor*500)*stage.Star, trial82FloorPoints(stage.Floor, stage.Star))
			if stage.Reward != oldReward && stage.Reward != newReward {
				return fmt.Errorf("%w: 试炼奖励偏离审核值 %s", constUtil.ErrAdminConfigConflict, stage.ID)
			}
			if stage.Reward != newReward {
				if err = tx.Model(&db.P7RushStageRule{}).Where("id=? AND reward=?", stage.ID, stage.Reward).Update("reward", newReward).Error; err != nil {
					return err
				}
				changed = true
			}
		}
		if !bytes.Equal(source.Rules, next.Rules) {
			// LiveEditable makes the operating source current while its historic frozen bytes remain evidence.
			if err = tx.Model(&db.P4CommerceRule{}).Where("id=?", source.ID).Update("rules", next.Rules).Error; err != nil {
				return err
			}
			changed = true
		}
		if _, err = s.growth75SyncShopWindows(tx, actor, map[string]db.P4CommerceRule{"point": next}); err != nil {
			return err
		}
		if changed {
			return adminAudit(tx, actor, constUtil.Trial82AuditAction, fmt.Sprint(s.OriginalServerID()), "商品六格加权刷新；神圣四部位5片/6000点；每层(10+层数)*星级，分段每星10点；资产及已领记录保持")
		}
		return nil
	})
}

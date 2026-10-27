package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ApplyActivity77Shops updates only approved trial requirements and adds stable
// weekly score bundles. Each catalogue uses the admin CAS and audit path.
func (s *GameService) ApplyActivity77Shops(ctx context.Context, actor *model.AdminPrincipal) error {
	var policy db.ClientUpdatePolicy
	if err := db.GetIns().WithContext(ctx).First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Maintenance {
		return fmt.Errorf("先关服再应用商店修复")
	}
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	var plan growth74Plan
	if err := json.Unmarshal(growth74Parameters, &plan); err != nil {
		return err
	}
	stars := map[string]int32{}
	for _, p := range plan.Stores["trial"].Products {
		threshold := int32(3)
		switch p.Menu {
		case 2:
			threshold = 30
		case 3:
			threshold = 60
		case 4:
			threshold = 90
		}
		stars[p.Reward] = threshold
	}
	materials := map[string]int32{"item_007": 3, "item_013": 3, "item_010": 6, "item_005": 9, "item_009": 12, "item_008": 18, "item_012": 21, "item_006": 24, "item_017": 90}
	for id, threshold := range materials {
		stars[id] = threshold
	}
	red := map[string]int32{"frag_equip_037": 135, "frag_equip_038": 165, "frag_equip_039": 135, "frag_equip_040": 195}
	for id, threshold := range red {
		stars[id] = threshold
	}
	point, err := s.AdminShop(ctx, "point")
	if err != nil {
		return err
	}
	_, err = s.adminShopMutation(ctx, actor, "point", point.Version, "activity77.stars", func(tx *gorm.DB, raw *adminShopStorage) error {
		if raw.Period == nil || raw.Period.Kind != "point" {
			return constUtil.ErrAdminConfiguration
		}
		var rules p4CommerceRules
		if err := json.Unmarshal(raw.Period.Rules, &rules); err != nil {
			return err
		}
		for i := range rules.Products {
			id, _, ok := strings.Cut(rules.Products[i].Rewards, ",")
			if !ok {
				return constUtil.ErrAdminConfiguration
			}
			if threshold, found := stars[id]; found {
				rules.Products[i].MinTrialStars = threshold
			}
		}
		rules.LiveEditable = true
		candidate := *raw.Period
		candidate.Rules = adminJSON(rules)
		if _, err := s.validateP4Commerce(candidate); err != nil {
			return err
		}
		return tx.Model(raw.Period).Update("rules", candidate.Rules).Error
	})
	if err != nil {
		return err
	}
	score, err := s.AdminShop(ctx, "score")
	if err != nil {
		return err
	}
	_, err = s.adminShopMutation(ctx, actor, "score", score.Version, "activity77.bundles", func(tx *gorm.DB, raw *adminShopStorage) error {
		if raw.Period == nil || raw.Period.Kind != "score" {
			return constUtil.ErrAdminConfiguration
		}
		var rules p4CommerceRules
		if err := json.Unmarshal(raw.Period.Rules, &rules); err != nil {
			return err
		}
		next := int32(1)
		seen := map[string]bool{}
		for _, p := range rules.Products {
			next = max(next, p.Index+1)
		}
		for _, amount := range []int{1000, 2000, 3000, 4000, 5000} {
			id := fmt.Sprintf("score_crystal77_%d", amount)
			wanted := p4Product{ID: id, Name: fmt.Sprintf("突破水晶周礼包·%d", amount), Index: next, Rewards: fmt.Sprintf("item_006,%d", amount), Cost: fmt.Sprintf("score,%d", amount*2), Priority: next, DailyLimit: 1, AccountWeeklyLimit: 1, Stock: -1, MinLevel: 35, Weight: 1, DiscountBps: 10000}
			enabled := true
			wanted.Enabled = &enabled
			found := false
			for _, existing := range rules.Products {
				if existing.ID == id {
					if seen[id] {
						return constUtil.ErrAdminConfiguration
					}
					seen[id] = true
					found = true
					if existing.Rewards != wanted.Rewards || existing.Cost != wanted.Cost || !existing.isEnabled() || existing.MinLevel != wanted.MinLevel || existing.MinVIP != 0 || existing.Stock != -1 || existing.DiscountBps != 10000 || existing.AccountWeeklyLimit != 1 || existing.DailyLimit != 1 || existing.TotalLimit != 0 || len(existing.QuantityTiers) != 0 {
						return fmt.Errorf("现有周礼包配置冲突: %s", id)
					}
				}
			}
			if !found {
				rules.Products = append(rules.Products, wanted)
				next++
			}
		}
		rules.LiveEditable = true
		candidate := *raw.Period
		candidate.Rules = adminJSON(rules)
		if _, err := s.validateP4Commerce(candidate); err != nil {
			return err
		}
		return tx.Model(raw.Period).Update("rules", candidate.Rules).Error
	})
	if err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		sources := map[string]db.P4CommerceRule{}
		for _, kind := range []string{"point", "score"} {
			source, e := s.AdminOrdinaryShopSource(tx, kind)
			if e != nil {
				return e
			}
			sources[kind] = source
		}
		_, err := s.growth75SyncShopWindows(tx, actor, sources)
		return err
	})
}

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func trialDailyGoodsEnabled(cfg db.P4CommerceRule, rules p4CommerceRules) bool {
	return cfg.Kind == "point" && cfg.ShopType == 5 && rules.TrialRefreshVersion == constUtil.TrialDailyGoodsVersion
}

func trialDailyProduct(cfg db.P4CommerceRule, rules p4CommerceRules, p p4Product) bool {
	return trialDailyGoodsEnabled(cfg, rules) && p.Priority >= 1000 && p.Priority < 2000
}

func trialDailyGoodsNext(now time.Time) int64 {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, local.Location()).UnixMilli()
}

// The original reward SKU and its lifetime history remain independent of the
// new daily SKU. Applying this configuration never rewrites player purchases.
func (s *GameService) trialDailyGoodsRules(rules p4CommerceRules) (p4CommerceRules, error) {
	if rules.TrialRefreshVersion == constUtil.TrialDailyGoodsVersion {
		return rules, nil
	}
	if rules.TrialRefreshVersion != 1 {
		return rules, errors.New("试炼每日商品需要现行刷新目录")
	}
	var accessories []p4Product
	var lastIndex int32
	liberation, sword := false, false
	for i := range rules.Products {
		p := &rules.Products[i]
		lastIndex = max(lastIndex, p.Index)
		if !p.isEnabled() {
			continue
		}
		reward, err := p1RewardResources(p.Rewards)
		if err != nil || len(reward) != 1 {
			return rules, errors.New("试炼每日商品需要单项奖励")
		}
		id := reward[0].GetId()
		if strings.HasPrefix(id, "frag_trea_") {
			accessories = append(accessories, *p)
			continue
		}
		if id == "frag_equip_037" && reward[0].GetNum() == 5 {
			if sword {
				return rules, errors.New("试炼神圣之剑碎片目录重复")
			}
			sword = true
			p.Priority = s.pointShop48Priority(p.Priority, reward)
			p.DailyLimit, p.TotalLimit = 1, 1
			continue
		}
		group, err := s.trialRefresh82Group(*p)
		if err != nil {
			return rules, err
		}
		if group != 1 {
			continue
		}
		if id == "item_005" {
			if liberation {
				enabled := false
				p.Enabled = &enabled
				continue
			}
			liberation = true
			p.Rewards = "item_005,250"
		}
		p.Priority = 1000 + p.Priority%1000
		p.TotalLimit, p.AccountWeeklyLimit = 0, 0
		p.TierID, p.QuantityTiers = false, nil
		switch id {
		case "item_005", "item_006", "item_008", "item_009", "item_010", "item_012":
			p.DailyLimit = 2
		}
	}
	if !sword || !liberation || len(accessories) == 0 {
		return rules, errors.New("试炼目录缺少神圣之剑、解放之石或饰品碎片")
	}
	for i, original := range accessories {
		p := original
		p.ID = constUtil.TrialDailyGoodsPrefix + p6Digest([]byte(original.ID))[:24]
		p.Index = lastIndex + int32(i) + 1
		p.Priority = 1500 + int32(i)
		reward, _ := p1RewardResources(original.Rewards)
		p.Rewards = reward[0].GetId() + ",1"
		p.Cost = fmt.Sprintf("point,%d", constUtil.TrialDailyGoodsPoints)
		p.MinTrialStars = constUtil.TrialDailyGoodsStars
		p.DailyLimit, p.TotalLimit, p.AccountWeeklyLimit = 1, 0, 0
		p.TierID, p.QuantityTiers = false, nil
		p.DiscountBps, p.Stock = 10000, -1
		rules.Products = append(rules.Products, p)
	}
	disabled := false
	rules.LiveEditable, rules.TrialRefreshVersion = true, constUtil.TrialDailyGoodsVersion
	rules.RefreshEnabled = &disabled
	rules.FreeCap, rules.FreeSeconds, rules.ResourceRefreshCap, rules.Slots = 0, 0, 0, 0
	rules.RefreshItem, rules.RefreshCost = "", ""
	return rules, nil
}

// ApplyTrialDailyGoods shares the existing operator authorization, catalogue
// version lock and audit transaction. It is an explicit configuration operation.
func (s *GameService) ApplyTrialDailyGoods(ctx context.Context, actor *model.AdminPrincipal, version string) (*model.AdminShopResponse, error) {
	return s.adminShopMutation(ctx, actor, "point", version, "trial.daily-goods", func(tx *gorm.DB, raw *adminShopStorage) error {
		if raw.Period == nil || raw.Period.Kind != "point" {
			return errors.New("试炼商店来源不可用")
		}
		var rules p4CommerceRules
		if err := json.Unmarshal(raw.Period.Rules, &rules); err != nil {
			return err
		}
		next, err := s.trialDailyGoodsRules(rules)
		if err != nil {
			return err
		}
		candidate := *raw.Period
		candidate.Rules = adminJSON(next)
		if _, err = s.validateP4Commerce(candidate); err != nil {
			return err
		}
		if bytes.Equal(raw.Period.Rules, candidate.Rules) {
			return nil
		}
		return tx.Model(raw.Period).Update("rules", candidate.Rules).Error
	})
}

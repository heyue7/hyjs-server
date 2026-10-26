package service

import (
	"context"
	"fmt"
	"hyjs-server/model"
	"strings"
)

func (s *GameService) growth74RedGear(id string, seen map[string]bool) bool {
	if seen[id] {
		return false
	}
	seen[id] = true
	if strings.HasPrefix(id, "frag_") {
		if r := s.table("Frag", "id", id); r != nil {
			return s.growth74RedGear(r["target"], seen)
		}
	}
	for _, table := range []string{"Equip", "Treasure"} {
		if r := s.table(table, "id", id); r != nil && tableNumber(r, "quality") >= 23 {
			return true
		}
	}
	if strings.HasPrefix(id, "item_") {
		for _, r := range s.Tables["ItemUse"] {
			if r["itemId"] == id {
				for _, part := range strings.Split(r["result"], ";") {
					if s.growth74RedGear(strings.Split(part, ",")[0], seen) {
						return true
					}
				}
			}
		}
	}
	return false
}
func (s *GameService) growth74Extra(ctx context.Context, a *model.AdminPrincipal) error {
	shop, e := s.AdminShop(ctx, "memory")
	if e != nil {
		return e
	}
	for _, v := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if len(v.Rewards) != 1 {
			continue
		}
		id := v.Rewards[0].ResourceID
		if strings.HasPrefix(id, "frag_hero_") {
			hero := s.table("Hero", "id", strings.TrimPrefix(id, "frag_"))
			quality := tableNumber(hero, "quality")
			if quality >= 23 && id != "frag_hero_001" && id != "frag_hero_003" && id != "frag_hero_005" {
				v.Enabled = false
			} else {
				switch {
				case quality >= 23:
					v.Rewards[0].Amount = 2
				case quality >= 20:
					v.Rewards[0].Amount = 5
				default:
					v.Rewards[0].Amount = 10
				}
				v.QuantityTiers = nil
			}
		} else {
			switch id {
			case "item_005", "item_006":
				v.Rewards[0].Amount = 50
			case "hero_exp_002":
				v.Rewards[0].Amount = 5
			case "item_013":
				v.Rewards[0].Amount = 2
			}
		}
		if s.growth74RedGear(id, map[string]bool{}) {
			v.Enabled = false
		}
		v.QuantityTiers = nil
		v.Source = "成长投放V12"
		shop, e = s.AdminSaveShopProduct(ctx, a, "memory", v.ID, model.AdminShopProductSaveRequest{Version: shop.Version, AdminShopProduct: v}, false)
		if e != nil {
			return fmt.Errorf("回忆圣坛 %s: %w", v.Name, e)
		}
	}
	// Retain native one-time VIP packs, placing the rainbow skill book at VIP7.
	vip, e := s.AdminShop(ctx, "vip")
	if e != nil {
		return e
	}
	found := false
	for _, v := range append([]model.AdminShopProduct(nil), vip.Items...) {
		r := []model.AdminResourceAmount{}
		for _, x := range v.Rewards {
			if !s.growth74RedGear(x.ResourceID, map[string]bool{}) && x.ResourceID != "item_063" {
				r = append(r, x)
			}
		}
		if v.MinVip == 7 {
			r = append(r, model.AdminResourceAmount{ResourceID: "item_063", Amount: 1})
			found = true
		}
		if len(r) == 0 {
			v.Enabled = false
		} else {
			v.Rewards = r
		}
		vip, e = s.AdminSaveShopProduct(ctx, a, "vip", v.ID, model.AdminShopProductSaveRequest{Version: vip.Version, AdminShopProduct: v}, false)
		if e != nil {
			return e
		}
	}
	if !found {
		v := model.AdminShopProduct{Name: "VIP7专属礼包", MinVip: 7, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 1}, Rewards: []model.AdminResourceAmount{{ResourceID: "item_063", Amount: 1}}, Enabled: true, Stock: -1, MinLevel: 1, DailyLimit: 1, DiscountBps: 10000, Weight: 1}
		if _, e = s.AdminSaveShopProduct(ctx, a, "vip", "", model.AdminShopProductSaveRequest{Version: vip.Version, AdminShopProduct: v}, true); e != nil {
			return e
		}
	}
	// Legacy recharge and festival entrances are retired; their immutable claims stay intact.
	native, e := s.AdminNativeActivities(ctx, a)
	if e != nil {
		return e
	}
	for _, v := range native {
		switch v.Kind {
		case "payment-first", "payment-single", "payment-total", "payment-continued", "payment-consume", "celebration", "festival", "update":
			if v.Enabled {
				if _, e = s.AdminSaveNativeActivity(ctx, a, v.ID, model.AdminNativeActivitySaveRequest{Version: v.Version, Enabled: false, Alias: v.Alias, Form: v.Form}); e != nil {
					return fmt.Errorf("停用旧活动 %s: %w", v.Name, e)
				}
			}
		}
	}
	// Any remaining non-credit/non-trial shop may retain ordinary materials only.
	for _, key := range []string{"popularity-rank", "pet-market", "union-fight"} {
		sh, err := s.AdminShop(ctx, key)
		if err != nil {
			return err
		}
		for _, v := range append([]model.AdminShopProduct(nil), sh.Items...) {
			bad := false
			for _, r := range v.Rewards {
				bad = bad || s.growth74RedGear(r.ResourceID, map[string]bool{})
			}
			if bad && v.Enabled {
				v.Enabled = false
				sh, err = s.AdminSaveShopProduct(ctx, a, key, v.ID, model.AdminShopProductSaveRequest{Version: sh.Version, AdminShopProduct: v}, false)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

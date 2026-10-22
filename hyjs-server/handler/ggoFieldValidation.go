package handler

import (
	"hyjs-server/model"
	"math"
	"strings"
)

// adminFieldValidGGO 限定设计配置引用和输入边界，配置不接入地图运行逻辑。
func adminFieldValidGGO(cfg *model.AdminFieldConfiguration) bool {
	g := cfg.GGO
	if g == nil {
		return true
	}
	if cfg.Scene != "field_003" || g.CurrencyID != "item_9710" || g.ShopKey != "ggo" || g.CarryIcon != "ggo_chip_marker" || g.SproutIcon != "ggo_sprout" || !ggoText(g.Title, 80) || !ggoText(g.ModeName, 80) {
		return false
	}
	if len(g.NPCs) != 4 || len(g.Monsters) != 5 || len(g.Bosses) != 3 || len(g.Chips) != 6 {
		return false
	}
	seen := map[string]bool{}
	models := map[string]bool{}
	for _, n := range g.NPCs {
		if !ggoIdentity(n.ID, seen) || models[n.HeroID] || !ggoText(n.Name, 80) || !ggoText(n.PointLabel, 120) || !ggoText(n.Dialogue, 1000) {
			return false
		}
		models[n.HeroID] = true
		guide := n.HeroID == "hero_005"
		if !guide && n.HeroID != "hero_029" && n.HeroID != "hero_073" && n.HeroID != "hero_053" {
			return false
		}
		if guide && (n.Role != "guide" || n.SettlesChips) || !guide && (n.Role != "settlement" || !n.SettlesChips) {
			return false
		}
		if p := n.Position; p != nil {
			for _, v := range []*float64{p.X, p.Y, p.Z} {
				if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
					return false
				}
			}
		}
	}
	for _, group := range []struct {
		values  []model.AdminFieldGGOEnemy
		allowed map[string]bool
	}{
		{g.Monsters, map[string]bool{"monster_026": true, "monster_029": true, "monster_031": true, "monster_032": true, "monster_003": true}},
		{g.Bosses, map[string]bool{"hero_017": true, "hero_016": true, "hero_006": true}},
	} {
		for _, e := range group.values {
			if !ggoIdentity(e.ID, seen) || !group.allowed[e.HeroID] || models[e.HeroID] || !ggoText(e.Name, 80) || e.RespawnMS < 1000 || e.RespawnMS > 604800000 {
				return false
			}
			models[e.HeroID] = true
		}
	}
	chips := map[string]bool{"item_9701": true, "item_9702": true, "item_9703": true, "item_9704": true, "item_9705": true, "item_9706": true}
	for _, c := range g.Chips {
		if !chips[c.ItemID] || !ggoIdentity(c.ItemID, seen) || !ggoText(c.Name, 80) || c.ExchangeValue != nil && (*c.ExchangeValue < 0 || *c.ExchangeValue > 2147483647) {
			return false
		}
	}
	for _, text := range []string{g.Extraction.Arrival, g.Extraction.Confirmation, g.Extraction.ConfirmButton, g.Extraction.ContinueButton, g.Extraction.Countdown, g.Extraction.Cancelled} {
		if !ggoText(text, 500) {
			return false
		}
	}
	return true
}
func ggoText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len([]rune(value)) <= limit
}
func ggoIdentity(id string, seen map[string]bool) bool {
	if !ggoText(id, 64) || seen[id] {
		return false
	}
	seen[id] = true
	return true
}

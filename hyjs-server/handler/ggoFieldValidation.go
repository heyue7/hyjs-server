package handler

import (
	"hyjs-server/constUtil"
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
	if cfg.Scene != "field_003" || g.SchemaVersion != constUtil.AdminFieldGGOSchemaVersion || g.CurrencyID != "item_9710" || g.ShopKey != "ggo" || g.CarryIcon != "ggo_chip_marker" || !ggoText(g.Title, 80) || !ggoText(g.ModeName, 80) {
		return false
	}
	if !adminFieldValidGGORules(g.Rules) {
		return false
	}
	if len(g.NPCs) != 5 || len(g.Monsters) != 5 || len(g.Bosses) != 3 || len(g.Chips) != 6 {
		return false
	}
	seen := map[string]bool{}
	models := map[string]bool{}
	roles := map[string]struct {
		heroID       string
		role         string
		settlesChips bool
	}{
		"shino": {heroID: "hero_005", role: "teleport"},
		"dane":  {heroID: "hero_029", role: "extraction"},
		"kuroe": {heroID: "hero_073", role: "extraction"},
		"serra": {heroID: "hero_053", role: "extraction"},
		"kohl":  {heroID: "clerk01_npc", role: "settlement", settlesChips: true},
	}
	for _, n := range g.NPCs {
		expected, ok := roles[n.ID]
		if !ok || !ggoIdentity(n.ID, seen) || models[n.HeroID] || n.HeroID != expected.heroID || n.Role != expected.role || n.SettlesChips != expected.settlesChips || !ggoText(n.Name, 80) || !ggoText(n.PointLabel, 120) || !ggoText(n.Dialogue, 1000) {
			return false
		}
		models[n.HeroID] = true
		if p := n.Position; p != nil {
			for _, v := range []*float64{p.X, p.Y, p.Z} {
				if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
					return false
				}
			}
		}
	}
	monsters := map[string]string{"scavenger": "monster_026", "assault": "monster_029", "raider": "monster_031", "guard": "monster_032", "worm": "monster_003"}
	bosses := map[string]string{"behemoth": "hero_017", "pale_rider": "hero_016", "death_gun": "hero_006"}
	for _, group := range []struct {
		values   []model.AdminFieldGGOEnemy
		expected map[string]string
	}{{g.Monsters, monsters}, {g.Bosses, bosses}} {
		for _, e := range group.values {
			heroID, ok := group.expected[e.ID]
			if !ok || e.HeroID != heroID || !ggoIdentity(e.ID, seen) || models[e.HeroID] || !ggoText(e.Name, 80) || e.RespawnMS < 1000 || e.RespawnMS > 604800000 {
				return false
			}
			models[e.HeroID] = true
		}
	}
	chips := map[string]bool{"item_9701": true, "item_9702": true, "item_9703": true, "item_9704": true, "item_9705": true, "item_9706": true}
	for _, c := range g.Chips {
		if !chips[c.ItemID] || !ggoIdentity(c.ItemID, seen) || !ggoText(c.Name, 80) || c.ExchangeValue != nil && (*c.ExchangeValue < 0 || *c.ExchangeValue > 2147483647) || c.DropWeight != nil && (*c.DropWeight < 0 || *c.DropWeight > 1000000) || c.DropAmount != nil && (*c.DropAmount < 1 || *c.DropAmount > 10000) {
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

// adminFieldValidGGORules 套用已确认政策，并允许仍待决定的数值保持为空。
func adminFieldValidGGORules(r model.AdminFieldGGORules) bool {
	if r.EntryMethod != "npc_random" || r.ProtectionSeconds < 0 || r.ProtectionSeconds > 300 || r.TerminalName != "临时终端" || !r.UnlimitedCapacity || r.SearchEnabled || r.InitialSource != "monsters" || r.PvpTransfer != "winner_terminal" || r.DeathReturn != "safe_area" || r.ExtractionReturn != "safe_area" || r.SettlementMode != "safe_npc_or_confirm_leave" || !r.RetainAfterDeath || r.CreditCap != "none" || r.ShopLimitScope != "account_weekly" || r.WeeklyResetTimezone != "Asia/Shanghai" {
		return false
	}
	if r.PveLossMode != "pending" && r.PveLossMode != "destroy" {
		return false
	}
	if r.TradeAssets != "pending" && r.TradeAssets != "chips_only" {
		return false
	}
	if r.WeeklyServerScope != "pending" && r.WeeklyServerScope != "current_server" && r.WeeklyServerScope != "all_servers" {
		return false
	}
	if !ggoOptionalIntInRange(r.DeathLossPercent, 0, 100) || !ggoOptionalIntInRange(r.ExtractionSeconds, 0, 300) || !ggoOptionalIntInRange(r.WeeklyLimit, 1, 10000) || !ggoOptionalIntInRange(r.WeeklyResetDay, 1, 7) || !ggoOptionalIntInRange(r.WeeklyResetHour, 0, 23) || !ggoOptionalIntInRange(r.WeeklyResetMinute, 0, 59) {
		return false
	}
	return ggoText(r.LeaveConfirmation, 500) && ggoText(r.LeaveConfirmButton, 80) && ggoText(r.LeaveCancelButton, 80)
}

func ggoOptionalIntInRange(value *int64, min, max int64) bool {
	return value == nil || *value >= min && *value <= max
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

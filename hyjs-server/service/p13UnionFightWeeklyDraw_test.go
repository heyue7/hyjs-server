package service

import (
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"testing"
)

func TestP13UFWeeklyDrawPrestigeUsesFrozenRuleNotTwoLosses(t *testing.T) {
	match := &db.P13UnionFightMatch{LeftGuildID: "left", RightGuildID: "right", StartAt: 100, EndAt: 1000}
	rule := &db.P13UnionFightSettlementPolicy{TerminalRule: constUtil.P13UFMixedTerminalRule, DrawPrestige: 5, WinPrestige: 50, LosePrestige: 25}
	result := &db.P13UnionFightSettlementResult{ResultKind: "draw", TerminalAt: 1000, EndAt: 1000, OfferHash: "frozen", Source: constUtil.P13UnionFightMixedSettlementSource, WinTaskPrestige: 3, LoseTaskPrestige: 4}
	for guild, extra := range map[string]int32{"left": 3, "right": 4} {
		base, task, err := p13UFMixedRoundPrestige(match, result, rule, guild)
		if err != nil || base != 5 || task != extra {
			t.Fatal("draw counted as win/loss", guild, base, task, err)
		}
	}
	rule.DrawPrestige = 0
	if _, _, err := p13UFMixedRoundPrestige(match, result, rule, "left"); err == nil {
		t.Fatal("unpublished draw prestige accepted")
	}
}

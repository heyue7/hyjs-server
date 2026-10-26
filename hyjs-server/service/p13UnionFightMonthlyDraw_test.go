package service

import (
	"hyjs-server/db"
	"testing"
)

func TestP13UFMonthlyLegacyWinsKeepOldPrestigeAndUnknownKindRefuses(t *testing.T) {
	match := &db.P13UnionFightMatch{LeftGuildID: "left", RightGuildID: "right", EndAt: 1000}
	rule := &db.P13UnionFightSettlementPolicy{WinPrestige: 50, LosePrestige: 25}
	result := &db.P13UnionFightSettlementResult{EndAt: 1000, OfferHash: "frozen", WinnerGuildID: "left", LoserGuildID: "right", WinTaskPrestige: 3, LoseTaskPrestige: 4}
	for guild, wanted := range map[string]int64{"left": 50, "right": 25} {
		base, _, err := p13UFMixedRoundPrestige(match, result, rule, guild)
		if err != nil || base != wanted {
			t.Fatal("legacy win/loss changed", guild, base, err)
		}
	}
	result.ResultKind = "unknown"
	if p13UFMixedRoundIdentity(match, result, rule) {
		t.Fatal("unknown result kind treated as loss")
	}
	result.ResultKind = "draw"
	result.WinnerGuildID = ""
	result.LoserGuildID = ""
	if p13UFMixedRoundIdentity(match, result, rule) {
		t.Fatal("legacy rule retroactively enables draw")
	}
}

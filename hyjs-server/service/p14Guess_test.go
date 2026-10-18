package service

import (
	"encoding/json"
	"strings"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func p14GuessPureOffer() *db.P14GuessOffer {
	rules := p14GuessRules{MinimumLevel: 30, OrdinaryCost: 10000, ChampionCost: 100000, OrdinaryRevert: 5000, ChampionRevert: 50000, OrdinarySuccess: 11000, ChampionSuccess: 110000, OrdinaryExtra: 10, ChampionExtra: 200, Settlement: "local-champion-payback-v1; certified-champion=successCoin+extraDiamond; other=guaranteedRevertCoin; certificate-required-no-early-award"}
	raw, _ := json.Marshal(rules)
	offer := &db.P14GuessOffer{SeasonIndex: 61, GuessType: 1, Version: "prepublished-local-v1", Source: constUtil.P14GuessSource, PublishedAt: 1, OpenAt: 1000, CloseAt: 2000, QualificationHash: strings.Repeat("a", 64), DrawHash: strings.Repeat("b", 64), Rules: raw, RulesHash: p6Digest(raw)}
	offer.SourceHash = p14GuessOfferHash(offer)
	return offer
}

// Recovered table numbers remain separate reward-pool previews. The combination is explicitly local and fixed before payment; no real
// champion or early award is manufactured from table field names.
func TestP14GuessOriginalTablePoolsAndLatePublicationBoundary(t *testing.T) {
	offer := p14GuessPureOffer()
	rules, err := p14GuessReadOffer(offer)
	if err != nil || rules.ChampionCost != 100000 || rules.ChampionSuccess != 110000 || rules.ChampionRevert != 50000 || rules.ChampionExtra != 200 {
		t.Fatal(err)
	}
	for _, change := range []func(*db.P14GuessOffer){
		func(o *db.P14GuessOffer) { o.PublishedAt = o.OpenAt + 1 },
		func(o *db.P14GuessOffer) { o.GuessType = 2 },
		func(o *db.P14GuessOffer) { o.QualificationHash = "" },
		func(o *db.P14GuessOffer) { o.Rules = []byte(`{"minimumLevel":30}`); o.RulesHash = p6Digest(o.Rules) },
	} {
		copy := *offer
		change(&copy)
		copy.SourceHash = p14GuessOfferHash(&copy)
		if _, err = p14GuessReadOffer(&copy); err == nil {
			t.Fatal("late/unproven/altered cost offer accepted")
		}
	}
}

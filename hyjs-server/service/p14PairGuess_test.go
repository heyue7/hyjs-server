package service

import (
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"strings"
	"testing"
)

// Identical local PIDs from different realms must stay in their actual pair;
// spectators can select either original entrant without collapsing identity.
func TestP14PairGuessActualJointPairIdentity(t *testing.T) {
	first := p14FinalDrawPair{Group: "A", Index: 0, LeftServerID: 1, LeftPlayerID: 1, RightServerID: 2, RightPlayerID: 1}
	second := p14FinalDrawPair{Group: "A", Index: 1, LeftServerID: 1, LeftPlayerID: 2, RightServerID: 2, RightPlayerID: 2}
	draw := &p14FinalDrawSnapshot{Pairs: []p14FinalDrawPair{first, second}}
	for _, identity := range [][2]int64{{1, 1}, {2, 1}} {
		actual, err := p14PairGuessFind(draw, identity[0], identity[1])
		if err != nil || *actual != first {
			t.Fatal("joint identity lost", err)
		}
	}
	if _, err := p14PairGuessFind(draw, 3, 1); err == nil {
		t.Fatal("unknown realm with same PID accepted")
	}
	draw.Pairs = append(draw.Pairs, first)
	if _, err := p14PairGuessFind(draw, 1, 1); err == nil {
		t.Fatal("ambiguous duplicated actual pair accepted")
	}
}

// Only the observed original state7 offer can consume a genuine first-round
// pair. Re-signed late publication or later-round labels cannot reuse that pair.
func TestP14PairGuessFrozenBeforeMatchOriginalPools(t *testing.T) {
	pair, _ := json.Marshal(p14FinalDrawPair{Group: "A", Index: 0, LeftServerID: 1, LeftPlayerID: 1, RightServerID: 2, RightPlayerID: 1})
	original := db.P14PairGuessOffer{SeasonIndex: 61, PairHash: p6Digest(pair), BattleType: 7, Pair: pair, DrawHash: strings.Repeat("a", 64), QualificationHash: strings.Repeat("b", 64), Version: "early-local-v1", Source: constUtil.P14PairGuessSource, PublishedAt: 100, OpenAt: 1000, CloseAt: 2000, CostCoin: 10000, RevertCoin: 5000, SuccessCoin: 11000, ExtraDiamond: 10, Settlement: constUtil.P14PairGuessSettlement}
	original.SourceHash = p14PairGuessOfferHash(&original)
	if err := p14PairGuessReadOffer(&original); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*db.P14PairGuessOffer){
		func(o *db.P14PairGuessOffer) { o.BattleType = 8 },
		func(o *db.P14PairGuessOffer) { o.BattleType = 9 },
		func(o *db.P14PairGuessOffer) { o.PublishedAt = o.OpenAt + 1 },
		func(o *db.P14PairGuessOffer) { o.CostCoin = 10001 },
		func(o *db.P14PairGuessOffer) { o.Settlement = "unknown-guessed-private-rule" },
		func(o *db.P14PairGuessOffer) { o.Pair = []byte(`{}`); o.PairHash = p6Digest(o.Pair) },
	} {
		altered := original
		change(&altered)
		altered.SourceHash = p14PairGuessOfferHash(&altered)
		if err := p14PairGuessReadOffer(&altered); err == nil {
			t.Fatal("unproven first-round offer accepted")
		}
	}
}

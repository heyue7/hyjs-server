package service

import (
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"math"
	"testing"
)

func TestAdminGuildConsumers31FrozenPlayerRules(t *testing.T) {
	legacy := p13UFPlayerBattle172Rules()
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	current := *legacy
	current.DurationMillis = 12500
	current.KillScore = 17
	current.KillToken = 23
	current.ArriveRange = 2.5
	current.ReviveSeconds = []int64{3, 9, 60}
	if !p13UFPlayerBattleRulesValid(&current) {
		t.Fatal("published operating values rejected")
	}
	var frozen p13UFPlayerBattleRules
	if err = json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	if !p13UFPlayerBattleRulesValid(&frozen) || frozen.DurationMillis != 5000 || frozen.KillScore != 5 || frozen.ReviveSeconds[0] != 5 {
		t.Fatal("legacy immutable snapshot changed or became unreadable")
	}
	for name, mutate := range map[string]func(*p13UFPlayerBattleRules){
		"duration":       func(r *p13UFPlayerBattleRules) { r.DurationMillis = 99 },
		"scoreNaN":       func(r *p13UFPlayerBattleRules) { r.KillScore = math.NaN() },
		"tokenInfinity":  func(r *p13UFPlayerBattleRules) { r.KillToken = math.Inf(1) },
		"range":          func(r *p13UFPlayerBattleRules) { r.ArriveRange = 101 },
		"reviveEmpty":    func(r *p13UFPlayerBattleRules) { r.ReviveSeconds = nil },
		"reviveOverflow": func(r *p13UFPlayerBattleRules) { r.ReviveSeconds = []int64{86401} },
		"engine":         func(r *p13UFPlayerBattleRules) { r.Engine = "unsupported" },
		"proof":          func(r *p13UFPlayerBattleRules) { r.CapabilityProof = "" },
		"rounds":         func(r *p13UFPlayerBattleRules) { r.MaxRounds = 41 },
		"factor":         func(r *p13UFPlayerBattleRules) { r.AttackerFactor = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := current
			mutate(&changed)
			if p13UFPlayerBattleRulesValid(&changed) {
				t.Fatal("invalid rules accepted")
			}
		})
	}
}

func TestAdminGuildConsumers31OperatingFiniteBounds(t *testing.T) {
	for _, tc := range []struct {
		value, low, high float64
		valid            bool
	}{
		{.01, .01, 100, true}, {100, .01, 100, true}, {.2, .01, 100, true},
		{.009, .01, 100, false}, {math.NaN(), .1, 100, false}, {math.Inf(1), .1, 100, false},
		{.8, .1, 100, true}, {2.5, .1, 100, true}, {101, .1, 100, false},
	} {
		if got := p13UFGuildFiniteRange(tc.value, tc.low, tc.high); got != tc.valid {
			t.Fatalf("value %v: got %v", tc.value, got)
		}
	}
}

func TestAdminGuildConsumers31FrozenQuoteBounds(t *testing.T) {
	for _, speed := range []float64{.2, 1.25} {
		q := db.P13UnionFightRevivePointOffer{MatchID: "old-match", PointID: 5, Radius: 3, Speed: speed, MaxStepMillis: 1000, MaxGapMillis: 2000, Source: "published"}
		q.PointGUID = p13UFFieldGUID(q.MatchID, "revive", "5")
		q.SourceHash = p13UFRevivePointOfferHash(&q)
		if !p13UFRevivePointQuoteValid(&q) {
			t.Fatal("legacy or updated speed rejected")
		}
		q.Speed = 101
		q.SourceHash = p13UFRevivePointOfferHash(&q)
		if p13UFRevivePointQuoteValid(&q) {
			t.Fatal("out of bounds speed accepted")
		}
	}
	for _, distance := range []float64{.8, 2.5} {
		q := db.P13UnionFightBattleAuditOffer{MatchID: "old-match", ArriveRange: distance, MaxRounds: 40, Source: "published"}
		q.SourceHash = p13UFBattleAuditOfferHash(&q)
		if !p13UFBattleAuditQuoteValid(&q) {
			t.Fatal("legacy or updated audit distance rejected")
		}
		q.MaxRounds = 41
		q.SourceHash = p13UFBattleAuditOfferHash(&q)
		if p13UFBattleAuditQuoteValid(&q) {
			t.Fatal("audit round guard relaxed")
		}
	}
	for _, cooldown := range []int64{5000, 12500} {
		q := db.P13UnionFightWarDrumOffer{ItemID: "ufitem_014", AttackPercent: 10, Radius: 3, DurationMillis: 1000, AuthorityMask: 7, CooldownMillis: cooldown, ModelName: constUtil.P13UnionFightWarDrumLocalModel, ModelScale: 1}
		q.SourceHash = p13UFDrumOfferHash(&q)
		if !p13UFDrumQuoteValid(&q) {
			t.Fatal("legacy or updated cooldown rejected")
		}
		q.CooldownMillis = 99
		q.SourceHash = p13UFDrumOfferHash(&q)
		if p13UFDrumQuoteValid(&q) {
			t.Fatal("out of bounds cooldown accepted")
		}
	}
}

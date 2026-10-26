package service

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"hyjs-server/db"
)

// Reuse the real registration/entry/mine/deposit/timed settlement/inbox flow.
// No battle outcome or reward row is manufactured by this regression.
func TestP13WorldSealedRealTerminalRepeatedWorkersPreserveCore(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, false, false)
	var results []db.P13UnionFightSettlementResult
	if err := db.GetIns().Where("delivered_at>0").Order("match_id").Limit(128).Find(&results).Error; err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("real delivered terminal missing")
	}
	models := []any{&db.P13UnionFightSettlementResult{}, &db.P13UnionFightFieldPlayer{}, &db.P13UnionFightFactoryState{}, &db.P13UnionFightFactoryCycleState{}, &db.P13UnionFightCarGeneration{}, &db.P13UnionFightCarAttackState{}, &db.P13UnionFightCarMotionState{}, &db.P13UnionFightTowerState{}, &db.P13UnionFightTowerHit{}, &db.P13UnionFightGuardState{}, &db.P13UnionFightGateState{}, &db.P13UnionFightResourceTask{}, &db.P13UnionFightTransferTask{}, &db.P13UnionFightHealAuraEffect{}, &db.P13UnionFightSpeedItemEffect{}, &db.P13UnionFightWarDrumEffect{}, &db.P13UnionFightReviveReceipt{}}
	read := func() string {
		t.Helper()
		all := []string{}
		for _, result := range results {
			for _, model := range models {
				var rows []map[string]any
				if err := db.GetIns().Model(model).Where("match_id=?", result.MatchID).Limit(129).Find(&rows).Error; err != nil {
					t.Fatal(err)
				}
				if len(rows) > 128 {
					t.Fatal("fixture exceeded bounded core snapshot")
				}
				encoded := []string{}
				for _, row := range rows {
					raw, err := json.Marshal(row)
					if err != nil {
						t.Fatal(err)
					}
					encoded = append(encoded, string(raw))
				}
				sort.Strings(encoded)
				raw, _ := json.Marshal(encoded)
				all = append(all, string(raw))
			}
		}
		raw, _ := json.Marshal(all)
		return string(raw)
	}
	before := read()
	// A cold service has no remembered writer/settlement state.
	s := &GameService{}
	workers := []struct {
		name string
		tick func(context.Context, time.Time) error
	}{
		{"factory", s.TickP13UnionFightFactory}, {"cycle", s.TickP13UnionFightFactoryCycle}, {"motion", s.TickP13UnionFightCarMotion}, {"car-attack", s.TickP13UnionFightCarAttack}, {"tower", s.TickP13UnionFightTower}, {"resource", s.TickP13UnionFightResource}, {"speed", s.TickP13UnionFightSpeedItem}, {"aura", s.TickP13UnionFightHealAura}, {"drum", s.TickP13UnionFightWarDrum}, {"focus", s.TickP13UnionFightFocus}, {"revive-point", s.TickP13UnionFightRevivePoint}, {"transfer", s.TickP13UnionFightTransfer}, {"guard-buildings", s.TickP13UnionFightGuardBuildings}, {"natural-revive", s.TickP13UnionFightNaturalRevive},
	}
	at := results[0].DeliveredAt
	for _, result := range results {
		if result.DeliveredAt > at {
			at = result.DeliveredAt
		}
	}
	for round := 0; round < 2; round++ {
		for _, worker := range workers {
			if err := worker.tick(context.Background(), time.UnixMilli(at)); err != nil {
				t.Fatalf("cold sealed %s: %v", worker.name, err)
			}
		}
		if read() != before {
			t.Fatal("sealed worker changed proof core")
		}
	}
}

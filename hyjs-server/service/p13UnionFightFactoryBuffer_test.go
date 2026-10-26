package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestP13UFFactoryBufferRuleLegacyHashesAndClock(t *testing.T) {
	for _, invalid := range []struct {
		rule          string
		buffer, alive int64
	}{{"", 1, 1}, {"unknown", 1, 1}, {constUtil.P13UnionFightFactoryBufferRule, 0, 1}, {constUtil.P13UnionFightFactoryBufferRule, 1, 2}} {
		if p13UFFactoryCapacityValid(invalid.rule, invalid.buffer, invalid.alive) {
			t.Fatal("unsupported capacity accepted", invalid)
		}
	}
	legacy := db.P13UnionFightFactoryCycleOffer{MatchID: "legacy", FactoryGUID: 4, CraftMillis: 120000, AnimationMillis: 10000, MaxGenerations: 5, MaxAlive: 2, Source: "legacy"}
	raw, _ := json.Marshal(legacy)
	if strings.Contains(string(raw), "buffer_capacity") || strings.Contains(string(raw), "rule") {
		t.Fatal("legacy quote hash bytes changed", string(raw))
	}
	oldOffer, _ := json.Marshal(struct {
		MatchID         string `json:"match_id"`
		FactoryGUID     int64  `json:"factory_guid"`
		CraftMillis     int64  `json:"craft_millis"`
		AnimationMillis int64  `json:"animation_millis"`
		MaxGenerations  int64  `json:"max_generations"`
		MaxAlive        int64  `json:"max_alive"`
		Source          string `json:"source"`
		SourceHash      string `json:"source_hash"`
	}{legacy.MatchID, legacy.FactoryGUID, legacy.CraftMillis, legacy.AnimationMillis, legacy.MaxGenerations, legacy.MaxAlive, legacy.Source, ""})
	if p13UFFactoryCycleOfferHash(&legacy) != p6Digest(oldOffer) {
		t.Fatal("legacy quote digest changed")
	}
	generation := db.P13UnionFightCarGeneration{MatchID: "legacy", CarGUID: 5, FactoryGUID: 4, Generation: 2, BornAt: 240000, FactorySnapshot: []byte("original"), OfferHash: "offer"}
	old, _ := json.Marshal(struct {
		MatchID                                  string
		CarGUID, FactoryGUID, Generation, BornAt int64
		FactorySnapshot                          []byte
		OfferHash                                string
	}{generation.MatchID, generation.CarGUID, generation.FactoryGUID, generation.Generation, generation.BornAt, generation.FactorySnapshot, generation.OfferHash})
	if p13UFCarGenerationHash(&generation) != p6Digest(old) {
		t.Fatal("legacy generation source changed")
	}
	frozen := &db.P13UnionFightFactoryState{StartedAt: 120000, AnimationAt: 230000, DueAt: 240000, CompletedAt: 310000}
	offer := &db.P13UnionFightFactoryCycleOffer{Rule: constUtil.P13UnionFightFactoryBufferRule, BufferCapacity: 1, MaxAlive: 1, CraftMillis: 120000}
	generation.BornAt = 310000
	generation.ProducedAt = 240000
	if err := p13UFCarGenerationClockValid(&generation, frozen, offer); err != nil {
		t.Fatal(err)
	}
	generation.BornAt = 230000
	if p13UFCarGenerationClockValid(&generation, frozen, offer) == nil {
		t.Fatal("buffer released before completion")
	}
	generation.BornAt = 310000
	generation.ProducedAt = 0
	if p13UFCarGenerationClockValid(&generation, frozen, offer) == nil {
		t.Fatal("new source omitted real production clock")
	}
	if err := p13UFCarGenerationClockValid(&generation, frozen, &legacy); err != nil {
		t.Fatal("legacy clock changed", err)
	}
}

func TestP13UFFactoryBufferGuardPausePreservesReadyAndRemaining(t *testing.T) {
	offer := &db.P13UnionFightFactoryCycleOffer{MatchID: "buffer", FactoryGUID: 7, Rule: constUtil.P13UnionFightFactoryBufferRule, BufferCapacity: 1, MaxAlive: 1, MaxGenerations: 5, CraftMillis: 120000}
	offer.SourceHash = p13UFFactoryCycleOfferHash(offer)
	factory := &db.P13UnionFightFactoryState{MatchID: "buffer", GUID: 7, CompletedAt: 100000}
	cycle := &db.P13UnionFightFactoryCycleState{MatchID: "buffer", FactoryGUID: 7, Generation: 2, ProductionStartedAt: 100000, NextBornAt: 220000, OfferHash: offer.SourceHash}
	cycle.StateHash = p13UFFactoryCycleStateHash(cycle)
	before := p13UFGuardFactoryTimerOf(factory, cycle)
	cycle.PausedAt = 160000
	cycle.RemainingMillis = 60000
	cycle.StateHash = p13UFFactoryCycleStateHash(cycle)
	after := p13UFGuardFactoryTimerOf(factory, cycle)
	row := &db.P13UnionFightGuardBattleAttempt{MatchID: "buffer", DueAt: 160000}
	if !p13UFGuardFactoryPauseProof(before, after, row, 7) {
		t.Fatal("active production pause rejected")
	}
	if err := p13UFGuardFactoryResumeCycle(cycle, 460000); err != nil {
		t.Fatal(err)
	}
	if cycle.ProductionStartedAt != 100000 || cycle.NextBornAt != 520000 {
		t.Fatal("pause lost real start/remainder", cycle)
	}
	cycle.BufferedAt = cycle.NextBornAt
	cycle.NextBornAt = 0
	cycle.StateHash = p13UFFactoryCycleStateHash(cycle)
	before = p13UFGuardFactoryTimerOf(factory, cycle)
	after = p13UFGuardFactoryTimerOf(factory, cycle)
	row.DueAt = 530000
	if !p13UFGuardFactoryPauseProof(before, after, row, 7) || !p13UFFactoryBufferStateValid(cycle, offer, 100000) {
		t.Fatal("ready buffer must survive guard defeat unchanged")
	}
	after.Cycle.NextBornAt = 900000
	after.Cycle.StateHash = p13UFFactoryCycleStateHash(after.Cycle)
	if p13UFGuardFactoryPauseProof(before, after, row, 7) {
		t.Fatal("guard fabricated another production timer")
	}
}

func TestP13UFFactoryBufferRealTimerColdReleaseAndEnd(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t, 600000)
	for _, model := range []any{&db.P13UnionFightFactorySource{}, &db.P13UnionFightFactoryCycleSource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarItemSource{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "guard_hp_source": "", "map_projection": "", "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "explicit isolated original field for real120s buffer fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightFactorySource{Day: match.Day, FactoryID: 1, Enabled: true, ChariotHP: 1000, PublishedAt: clock.Load(), Source: "isolated real120s buffer fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightFactoryCycleSource{Day: match.Day, FactoryID: 1, Enabled: true, MaxGenerations: 4, MaxAlive: 1, Rule: constUtil.P13UnionFightFactoryBufferRule, BufferCapacity: 1, PublishedAt: clock.Load(), Source: "explicit one live one buffer fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.GetIns().Where("day=? AND factory_id=1", match.Day).Delete(&db.P13UnionFightFactoryCycleSource{})
		db.GetIns().Where("day=? AND factory_id=1", match.Day).Delete(&db.P13UnionFightFactorySource{})
	})
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	tick := func(at int64) {
		t.Helper()
		clock.Store(at)
		if err := f.s.TickP13UnionFightFactory(f.ctx, time.UnixMilli(at)); err != nil {
			t.Fatal(err)
		}
	}
	guid := p13UFFieldGUID(match.ID, "factory", "1")
	read := func() db.P13UnionFightFactoryCycleState {
		t.Helper()
		var row db.P13UnionFightFactoryCycleState
		if err := db.GetIns().First(&row, "match_id=? AND factory_guid=?", match.ID, guid).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	count := func(want int64) {
		t.Helper()
		var n int64
		if err := db.GetIns().Model(&db.P13UnionFightCarGeneration{}).Where("match_id=?", match.ID).Count(&n).Error; err != nil || n != want {
			t.Fatal(n, want, err)
		}
	}
	tick(first + 120000)
	tick(first + 240000)
	state := read()
	count(0)
	if state.BufferedAt != first+240000 || state.NextBornAt != 0 || state.Generation != 2 {
		t.Fatal("full alive slot lost completed buffer", state)
	}
	f.open(t)
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	tick(first + 300000)
	count(0)
	if read().BufferedAt != state.BufferedAt {
		t.Fatal("cold worker rewrote completed buffer")
	}
	var factory db.P13UnionFightFactoryState
	if err := db.GetIns().First(&factory, "match_id=? AND guid=?", match.ID, guid).Error; err != nil {
		t.Fatal(err)
	}
	car, err := p13UFCarItemRead(&factory)
	if err != nil {
		t.Fatal(err)
	}
	// Controlled low-level death input; this fixture asserts no damage authority or reward.
	car.Hp = proto.Float64(0)
	factory.CarFrame, _ = proto.Marshal(car)
	factory.CarHash = p6Digest(factory.CarFrame)
	if err = p13UFCarSave(db.GetIns(), &factory); err != nil {
		t.Fatal(err)
	}
	tick(first + 310000)
	count(1)
	var born db.P13UnionFightCarGeneration
	if err = db.GetIns().First(&born, "match_id=? AND factory_guid=? AND generation=2", match.ID, guid).Error; err != nil {
		t.Fatal(err)
	}
	if born.ProducedAt != first+240000 || born.BornAt != first+310000 || born.CarGUID == car.GetGuid() {
		t.Fatal("buffer forged historical world birth", born)
	}
	state = read()
	if state.BufferedAt != 0 || state.NextBornAt != first+430000 || state.ProductionStartedAt != first+310000 {
		t.Fatal("next craft did not start at actual release", state)
	}
	tick(first + 310000)
	count(1)
	tick(first + 430000)
	count(1)
	if read().BufferedAt != first+430000 {
		t.Fatal("second complete buffer missing")
	}
	tick(match.EndAt)
	count(1)
	if read().BufferedAt != first+430000 {
		t.Fatal("terminal erased or released buffer")
	}
}

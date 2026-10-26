package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestP13UFWorldClockEffectiveEndAndBarrierAreIndependent(t *testing.T) {
	match := &db.P13UnionFightMatch{ID: "scope", EndAt: 100000}
	clocks := &p13UFWorldClocks{terminal: map[string]int64{}, barrier: map[string]int64{"scope": 50000}}
	if !clocks.paused("scope") || clocks.end(match) != 100000 {
		t.Fatal("barrier must pause without inventing a terminal")
	}
	clocks.barrier = map[string]int64{}
	if clocks.paused("scope") || clocks.end(match) != 100000 {
		t.Fatal("failed base resumes original frozen clock")
	}
	clocks.terminal["scope"] = 50000
	if clocks.end(match) != 50000 || match.EndAt != 100000 {
		t.Fatal("actual terminal must not rewrite frozen match")
	}
	clocks.terminal["scope"] = 150000
	if clocks.end(match) != 100000 {
		t.Fatal("terminal may never extend match")
	}
}

func TestP13UFWorldClockBatchCanceledContext(t *testing.T) {
	p13UFFieldSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p13UFWorldClockBatch(ctx, []string{"scope"}, time.UnixMilli(10000)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestP13UFWorldFactoryTimerLegacyHashAndLogicalResume(t *testing.T) {
	factory := db.P13UnionFightFactoryState{MatchID: "clock-fixture", GUID: 9, OpenedAt: 1, StartedAt: 1000, AnimationAt: 111000, DueAt: 121000, FrameHash: "frame", CarHash: "car"}
	old, _ := json.Marshal(struct {
		MatchID                                                    string
		GUID, OpenedAt, StartedAt, AnimationAt, DueAt, CompletedAt int64
		FrameHash, CarHash                                         string
	}{factory.MatchID, factory.GUID, factory.OpenedAt, factory.StartedAt, factory.AnimationAt, factory.DueAt, factory.CompletedAt, factory.FrameHash, factory.CarHash})
	if p13UFFactoryStateDigest(&factory) != p6Digest(old) {
		t.Fatal("zero optional fields changed old hash")
	}
	static := p13UFFactoryDigest(&factory)
	before := p13UFGuardFactoryTimerOf(&factory, nil)
	factory.PausedAt = 61000
	factory.RemainingMillis = 60000
	after := p13UFGuardFactoryTimerOf(&factory, nil)
	row := &db.P13UnionFightGuardBattleAttempt{MatchID: factory.MatchID, DueAt: 61000}
	if !p13UFGuardFactoryPauseProof(before, after, row, factory.GUID) {
		t.Fatal("real remaining craft rejected")
	}
	after.RemainingMillis++
	if p13UFGuardFactoryPauseProof(before, after, row, factory.GUID) {
		t.Fatal("manufactured remaining craft accepted")
	}
	// Restart at any later wall-clock must resume from logical recovery361000,
	// rather than delaying another120sec from the observation time.
	if err := p13UFGuardFactoryResumeTimer(&factory, 361000); err != nil {
		t.Fatal(err)
	}
	if factory.DueAt != 421000 || factory.AnimationAt != 411000 || factory.StartedAt != 1000 || factory.PausedAt != 0 || factory.RemainingMillis != 0 || p13UFFactoryDigest(&factory) != static {
		t.Fatal(factory)
	}
	if err := p13UFGuardFactoryResumeTimer(&factory, 361000); err == nil {
		t.Fatal("duplicate resume accepted")
	}
	cycle := db.P13UnionFightFactoryCycleState{MatchID: "clock-fixture", FactoryGUID: 9, Generation: 7, NextBornAt: 121000, OfferHash: "frozen", PausedAt: 61000, RemainingMillis: 60000}
	if err := p13UFGuardFactoryResumeCycle(&cycle, 361000); err != nil || cycle.NextBornAt != 421000 || cycle.Generation != 7 || cycle.OfferHash != "frozen" {
		t.Fatal(cycle, err)
	}
}

func TestP13UFWorldTowerCutoffIsNoEffectAndImmutable(t *testing.T) {
	tower := p13UFTowerFixtureState()
	tower.Sequence = 1
	tower.StateHash = p13UFTowerStateHash(tower)
	hit := &db.P13UnionFightTowerHit{MatchID: tower.MatchID, TowerGUID: tower.TowerGUID, Sequence: 1, TargetKind: "car", TargetGUID: 99, Status: constUtil.P13UnionFightTowerHitCanceled, CancelReason: p13UFTowerCancelWorld, HitAt: 10600, DueAt: 10600, AppliedAt: 11000, CutoffAt: 10500, TowerSourceHash: tower.SourceHash}
	hit.ImmutableHash = p13UFTowerImmutableHash(hit)
	hit.HitHash = p13UFTowerHitHash(hit)
	if err := p13UFTowerHitVerify(hit, tower); err != nil {
		t.Fatal(err)
	}
	immutable := hit.ImmutableHash
	hit.CutoffAt = hit.HitAt
	hit.HitHash = p13UFTowerHitHash(hit)
	if p13UFTowerImmutableHash(hit) != immutable {
		t.Fatal("resolution changed immutable launch")
	}
	if p13UFTowerHitVerify(hit, tower) == nil {
		t.Fatal("in-window shot falsely canceled")
	}
	hit.CutoffAt = 10500
	hit.Before = []byte("invented damage")
	hit.HitHash = p13UFTowerHitHash(hit)
	if p13UFTowerHitVerify(hit, tower) == nil {
		t.Fatal("cutoff cancellation carried HP effect")
	}
}

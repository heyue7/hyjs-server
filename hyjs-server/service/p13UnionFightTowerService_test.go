package service

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFTowerFixtureState() *db.P13UnionFightTowerState {
	row := &db.P13UnionFightTowerState{MatchID: strings.Repeat("a", 64), TowerID: 1, GuardGUID: 123, Camp: 0, Range: 1.4, HPPercent: 50, CooldownMillis: 5000, ReleaseMillis: 600, AnimationMillis: 1000, ReviveSeconds: constUtil.P13UnionFightTowerReviveSeconds, MatchHash: strings.Repeat("b", 64), Version: "isolated-test-v1", PublishedAt: 1, Source: "explicit test50percent, never production", Active: true, NextFireAt: 1000}
	row.TowerGUID = p13UFFieldGUID(row.MatchID, "tower", "1")
	row.SourceHash = p13UFTowerSourceHash(row)
	row.StateHash = p13UFTowerStateHash(row)
	return row
}
func p13UFTowerFixturePlayer() *pb.InstUFPlayer {
	return &pb.InstUFPlayer{Guid: proto.Int64(900), Camp: proto.Int32(1), State: proto.Int32(0), KillNum: proto.Int32(3), DeadNum: proto.Int32(0), OwnScore: proto.Float64(15), OwnToken: proto.Float64(22), FightInfo: &pb.InstFightable{Guid: proto.Int64(900), Index: []int32{0, 1, 2, 3}, HpPos: []float64{100, 50, 0, 200}, MaxHpPos: []float64{100, 100, 80, 200}, Anger: proto.Float64(7)}}
}
func TestP13UnionFightTowerLegacyFourSlotsProofNoRewards(t *testing.T) {
	before := p13UFTowerFixturePlayer()
	after, revive, err := p13UFTowerDamagePlayer(before, 50)
	if err != nil || revive != 0 || after.FightInfo.HpPos[0] != 50 || after.FightInfo.HpPos[1] != 0 || after.FightInfo.HpPos[2] != 0 || after.FightInfo.HpPos[3] != 100 {
		t.Fatal("each surviving actor must lose its own MaxHP fraction", after, err)
	}
	dead, revive, err := p13UFTowerDamagePlayer(after, 50)
	if err != nil || revive != 5 || dead.GetState() != 3 || dead.GetDeadNum() != 1 {
		t.Fatal("all actors must be zero before environmental death", dead, err)
	}
	if before.GetDeadNum() != 0 || before.FightInfo.HpPos[0] != 100 || dead.GetKillNum() != 3 || dead.GetOwnScore() != 15 || dead.GetOwnToken() != 22 || dead.FightInfo.GetAnger() != 7 {
		t.Fatal("source or player rewards changed")
	}
	before.DeadNum = proto.Int32(19)
	_, revive, err = p13UFTowerDamagePlayer(before, 100)
	if err != nil || revive != 40 {
		t.Fatal("original ordinal revival must cap at40", revive, err)
	}
	before.State = proto.Int32(1)
	if _, _, err = p13UFTowerDamagePlayer(before, 50); err == nil {
		t.Fatal("in-combat target accepted")
	}
	before.State = proto.Int32(0)
	before.FightInfo.MaxHpPos[0] = 0
	if _, _, err = p13UFTowerDamagePlayer(before, 50); err == nil {
		t.Fatal("invalid frozen HP accepted")
	}
}
func TestP13UnionFightTowerDeterministicCarPriorityAndReleaseEligibility(t *testing.T) {
	tower := p13UFTowerFixtureState()
	player := &p13UFTowerTarget{kind: "player", guid: 1, camp: 1, x: .1, player: &db.P13UnionFightFieldPlayer{}, frame: p13UFTowerFixturePlayer()}
	farCar := &p13UFTowerTarget{kind: "car", guid: 5, camp: 1, x: 1, car: &pb.InstUFCariot{Hp: proto.Float64(1)}}
	tieCar := &p13UFTowerTarget{kind: "car", guid: 4, camp: 1, x: 1, car: &pb.InstUFCariot{Hp: proto.Float64(1)}}
	if target := p13UFTowerSelect(tower, []*p13UFTowerTarget{farCar, player, tieCar}); target != tieCar {
		t.Fatal("car priority and stable GUID tie")
	}
	tieCar.x = 2
	farCar.car.Hp = proto.Float64(0)
	if target := p13UFTowerSelect(tower, []*p13UFTowerTarget{farCar, player, tieCar}); target != player {
		t.Fatal("out of range/dead car accepted")
	}
	player.frame.State = proto.Int32(1)
	if p13UFTowerSelect(tower, []*p13UFTowerTarget{player}) != nil {
		t.Fatal("busy player target")
	}
	player.frame.State = proto.Int32(0)
	player.player.LeftAt = 1
	if p13UFTowerSelect(tower, []*p13UFTowerTarget{player}) != nil {
		t.Fatal("departed player target")
	}
}
func TestP13UnionFightTowerHitRejectsForgedRewardsAndHashAndIdleNoCatchup(t *testing.T) {
	tower := p13UFTowerFixtureState()
	tower.Sequence = 1
	tower.StateHash = p13UFTowerStateHash(tower)
	before := p13UFTowerFixturePlayer()
	after, revive, err := p13UFTowerDamagePlayer(before, 100)
	if err != nil {
		t.Fatal(err)
	}
	tower.HPPercent = 100
	tower.SourceHash = p13UFTowerSourceHash(tower)
	tower.StateHash = p13UFTowerStateHash(tower)
	hit := &db.P13UnionFightTowerHit{MatchID: tower.MatchID, TowerGUID: tower.TowerGUID, Sequence: 1, TargetKind: "player", TargetGUID: 900, DueAt: 1600, AppliedAt: 2000, DeadPlayerID: 900, ReviveSeconds: revive, ReviveAt: 1600 + revive*1000, TowerSourceHash: tower.SourceHash}
	after.LeftTime = proto.Float32(4.6)
	hit.Before, _ = proto.Marshal(before)
	hit.After, _ = proto.Marshal(after)
	hit.BeforeHash = p6Digest(hit.Before)
	hit.AfterHash = p6Digest(hit.After)
	hit.HitHash = p13UFTowerHitHash(hit)
	if err = p13UFTowerHitVerify(hit, tower); err != nil {
		t.Fatal("real small environmental proof", err)
	}
	after.KillNum = proto.Int32(after.GetKillNum() + 1)
	hit.After, _ = proto.Marshal(after)
	hit.AfterHash = p6Digest(hit.After)
	hit.HitHash = p13UFTowerHitHash(hit)
	if p13UFTowerHitVerify(hit, tower) == nil {
		t.Fatal("reward-changing proof accepted despite recomputed hashes")
	}
	tower.PendingReleaseAt = 1600
	tower.PendingTargetGUID = 900
	tower.PendingTargetKind = "player"
	tower.NextFireAt = 1600
	sourceHash := tower.SourceHash
	p13UFTowerClear(tower, 100000)
	tower.StateHash = p13UFTowerStateHash(tower)
	if !p13UFTowerValid(tower) || tower.Sequence != 1 || tower.NextFireAt != 104400 || tower.PendingReleaseAt != 0 || tower.SourceHash != sourceHash {
		t.Fatal("late release caught up missed shots or changed frozen source", tower)
	}
	tower.HPPercent = 51
	if p13UFTowerValid(tower) {
		t.Fatal("source changed without matching frozen digest")
	}
}
func TestP13UnionFightTowerCarDamagePreservesIdentityAndClamps(t *testing.T) {
	before := &pb.InstUFCariot{Guid: proto.Int64(51), FactoryGuid: proto.Int64(9), Camp: proto.Int32(1), Hp: proto.Float64(10), MaxHp: proto.Float64(200)}
	after, err := p13UFTowerDamageCar(before, 20)
	if err != nil || after.GetHp() != 0 || after.GetGuid() != 51 || after.GetFactoryGuid() != 9 || before.GetHp() != 10 {
		t.Fatal("actual car damage", after, err)
	}
}

// This uses real persisted field/player identities. ATT65 / car5percent is an explicit
// isolated test publication; this test creates no production policy defaults.
func TestP13UnionFightTowerPersistedReleaseCancelLateDeathAndIdempotence(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if err := f.s.InitializeP13UnionFightTower(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&db.P13UnionFightTowerSource{}, &db.P13UnionFightTowerState{}, &db.P13UnionFightTowerHit{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated tower test map fixture only"}).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightTowerSource{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("initialization published a damage default", count, err)
	}
	for id := int64(1); id <= 6; id++ {
		if err := db.GetIns().Create(&db.P13UnionFightTowerSource{Day: match.Day, TowerID: id, Enabled: true, Version: "isolated-tower-physical-v1", HPPercent: 5, PlayerAttack: 65, PlayerDamageRule: constUtil.P13UnionFightTowerPlayerDamageRule, PublishedAt: clock.Load(), Source: "explicit isolated local physical ATT65 / car5percent"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.s.RegisterP10Client(f.ctx, ids[0], profile.Session, func(proto.Message) error { return nil }))
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
		t.Fatal(err)
	}
	// The integration owner wires Build into field creation. Permit isolated
	// execution before that shared hook is merged, using the same real snapshot.
	if err := db.GetIns().Model(&db.P13UnionFightTowerState{}).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		snapshot, err := p13UFFieldStoredProjection(db.GetIns(), match.ID)
		if err != nil {
			t.Fatal(err)
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if err = proto.Unmarshal(snapshot.Buildings, buildings); err != nil {
			t.Fatal(err)
		}
		original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
		if err != nil {
			t.Fatal(err)
		}
		if err = f.s.p13UFTowerBuild(db.GetIns(), match, buildings, original, now()); err != nil {
			t.Fatal(err)
		}
	}
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	var tower db.P13UnionFightTowerState
	if err := db.GetIns().Where("match_id=? AND camp<>?", match.ID, actor.Camp).Order("tower_id").First(&tower).Error; err != nil {
		t.Fatal(err)
	}
	actor.X, actor.Y, actor.Z = tower.X, tower.Y, tower.Z
	if err := db.GetIns().Save(&actor).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(first)
	if err := f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&tower, "match_id=? AND tower_guid=?", match.ID, tower.TowerGUID).Error; err != nil || tower.PendingTargetGUID != ids[0] {
		t.Fatal("real launch target", tower, err)
	}
	clock.Store(first + 599)
	if err := f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	db.GetIns().Model(&db.P13UnionFightTowerHit{}).Where("match_id=?", match.ID).Count(&count)
	if count != 0 {
		t.Fatal("damage before release")
	}
	frame, err := p13UFFieldReadActor(&actor)
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly move out of range before impact; busy players are now
	// covered by the independent real-occupancy deferred test below.
	actor.X = tower.X + 100
	if err = p13UFFieldSaveActor(db.GetIns(), &actor, frame); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 600)
	if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	db.GetIns().Model(&db.P13UnionFightTowerHit{}).Where("match_id=?", match.ID).Count(&count)
	if count != 1 {
		t.Fatal("out-of-range projectile cancellation was not preserved", count)
	}
	actor.X = tower.X
	frozen, damage, freezeErr := p13UFTowerFreezeActor(&actor, frame, &tower)
	if freezeErr != nil || frozen == nil || damage*1.5 > frame.FightInfo.MaxHpPos[0] {
		t.Fatal("explicit low-HP release fixture", freezeErr)
	}
	// Limited initial HP fixture for two ambient hits, not a claimed battle result.
	frame.FightInfo.HpPos[0] = damage * 1.5
	if err = p13UFFieldSaveActor(db.GetIns(), &actor, frame); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 5000)
	if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 5600)
	if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 10000)
	if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	// Reopen the same isolated database between launch and release. No new
	// Build/publication occurs, and only the accepted pending projectile survives.
	sqlDB, openErr := db.GetIns().DB()
	if openErr != nil {
		t.Fatal(openErr)
	}
	if openErr = sqlDB.Close(); openErr != nil {
		t.Fatal(openErr)
	}
	f.open(t)
	t.Cleanup(f.s.RegisterP10Client(f.ctx, ids[0], profile.Session, func(proto.Message) error { return nil }))
	clock.Store(first + 60000)
	if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var hits []db.P13UnionFightTowerHit
	if err = db.GetIns().Where("match_id=?", match.ID).Order("sequence").Find(&hits).Error; err != nil || len(hits) != 3 {
		t.Fatal("must retain one canceled and only two applied shots", len(hits), err)
	}
	if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	after, err := p13UFFieldReadActor(&actor)
	if err != nil || after.GetState() != 3 || after.GetDeadNum() != frame.GetDeadNum()+1 || after.GetKillNum() != frame.GetKillNum() || after.GetOwnScore() != frame.GetOwnScore() || after.GetOwnToken() != frame.GetOwnToken() {
		t.Fatal("environment death or invented rewards", after, err)
	}
	if err = db.GetIns().First(&tower, "match_id=? AND tower_guid=?", match.ID, tower.TowerGUID).Error; err != nil {
		t.Fatal(err)
	}
	applied := 0
	for _, hit := range hits {
		if hit.Status == constUtil.P13UnionFightTowerHitApplied {
			applied++
		}
	}
	if applied != 2 {
		t.Fatal("late worker fabricated additional damage", applied)
	}
	for i := range hits {
		if err = p13UFTowerHitVerify(&hits[i], &tower); err != nil {
			t.Fatal("immutable hit proof", err)
		}
	}
}

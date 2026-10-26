package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func p13TowerPhysicalFixture(t *testing.T) (*db.P13UnionFightTowerState, *db.P13UnionFightFieldPlayer, *pb.InstUFPlayer, *p13UFTowerTarget) {
	t.Helper()
	tower := p13UFTowerFixtureState()
	tower.MatchID = p6Digest([]byte(t.Name()))
	tower.TowerGUID = p13UFFieldGUID(tower.MatchID, "tower", "1")
	tower.HPPercent = 5
	tower.PlayerAttack = 65
	tower.PlayerDamageRule = constUtil.P13UnionFightTowerPlayerDamageRule
	tower.Sequence = 2
	tower.SourceHash = p13UFTowerSourceHash(tower)
	tower.StateHash = p13UFTowerStateHash(tower)
	frame := p13UFTowerFixturePlayer()
	frame.FightInfo.Index = []int32{1, 2, 3, 4}
	party := &pb.BattlePlayer{PlayerId: proto.Int64(frame.GetGuid())}
	for i, pose := range frame.FightInfo.Index {
		party.Formation = append(party.Formation, &pb.BattleHero{Guid: proto.String("frozen-actor-" + string(rune('a'+i))), Id: proto.String("hero_047"), Pose: proto.Int32(pose), Level: proto.Int32(1), BloodNumber: proto.Int32(1), Prop: &pb.BaseProp{Hp: proto.Float64(frame.FightInfo.MaxHpPos[i]), Pdef: proto.Float64(20), DamageReduce: proto.Float64(20)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(frame.FightInfo.HpPos[i])}})
	}
	raw, err := proto.Marshal(party)
	if err != nil {
		t.Fatal(err)
	}
	actor := &db.P13UnionFightFieldPlayer{MatchID: tower.MatchID, PlayerID: frame.GetGuid(), Camp: 1, Battle: raw, BattleHash: p6Digest(raw)}
	target := &p13UFTowerTarget{kind: "player", guid: frame.GetGuid(), camp: 1, player: actor, frame: frame, occupancyKind: constUtil.P13UnionFightTowerOccupancyOrdinary, occupancyID: strings.Repeat("d", 64), occupancyInputHash: strings.Repeat("e", 64), occupancyDueAt: 5000}
	return tower, actor, frame, target
}
func TestP13TowerPhysicalSingleSlotDefenseGrowthAndLegacyHash(t *testing.T) {
	legacy := p13UFTowerFixtureState()
	if legacy.SourceHash != "ad74c4d54d70d23e2bc841e7e7245f708373a33d4f443f99b2c0df3f769a5dba" {
		t.Fatal("new optional fields changed legacy source bytes", legacy.SourceHash)
	}
	tower, actor, before, _ := p13TowerPhysicalFixture(t)
	hero, damage, err := p13UFTowerFreezeActor(actor, before, tower)
	if err != nil || damage != 36 {
		t.Fatal("local (65-20)*(1-.20)", damage, err)
	}
	after, revive, err := p13UFTowerDamageBoundActor(before, hero, damage)
	if err != nil || revive != 0 || after.FightInfo.HpPos[0] != 64 || after.FightInfo.HpPos[1] != 50 || after.FightInfo.HpPos[2] != 0 || after.FightInfo.HpPos[3] != 200 || before.FightInfo.HpPos[0] != 100 {
		t.Fatal("tower damaged another slot / source", after, err)
	}
	hero.Prop.Pdef = proto.Float64(60)
	got, err := p13UFTowerPhysicalDamage(hero, 65)
	if err != nil || got != 4 {
		t.Fatal("defense growth not effective", got, err)
	}
	hero.Prop.Pdef = proto.Float64(10000)
	got, err = p13UFTowerPhysicalDamage(hero, 65)
	if err != nil || got != 1 {
		t.Fatal("original max1 clamp", got, err)
	}
	if after.GetKillNum() != before.GetKillNum() || after.GetOwnScore() != before.GetOwnScore() || after.GetOwnToken() != before.GetOwnToken() || after.FightInfo.GetAnger() != 7 {
		t.Fatal("invented reward or AP")
	}
	before.State = proto.Int32(1)
	target := &p13UFTowerTarget{kind: "player", guid: 900, camp: 1, player: actor, frame: before}
	if p13UFTowerSelect(tower, []*p13UFTowerTarget{target}) == nil {
		t.Fatal("busy immunity remains")
	}
}

func p13TowerCaptureFixture(t *testing.T, tx *gorm.DB, tower *db.P13UnionFightTowerState, target *p13UFTowerTarget, sequence, hitAt, now int64) *db.P13UnionFightTowerHit {
	t.Helper()
	target.frame.State = proto.Int32(1)
	tower.Sequence = sequence
	tower.PendingTargetGUID = target.guid
	tower.PendingTargetKind = "player"
	tower.PendingReleaseAt = hitAt
	tower.NextFireAt = hitAt
	if err := p13UFTowerFreezeLaunch(tower, target); err != nil {
		t.Fatal(err)
	}
	tower.StateHash = p13UFTowerStateHash(tower)
	row := &db.P13UnionFightTowerHit{MatchID: tower.MatchID, TowerGUID: tower.TowerGUID, Sequence: sequence, TargetKind: "player", TargetGUID: target.guid, HitAt: hitAt, DueAt: hitAt, AppliedAt: now, TowerSourceHash: tower.SourceHash, Status: constUtil.P13UnionFightTowerHitApplied}
	if err := p13UFTowerCapturePhysical(tx, row, tower, target); err != nil {
		t.Fatal(err)
	}
	row.HitHash = p13UFTowerHitHash(row)
	if err := p13UFTowerHitVerify(row, tower); err != nil {
		t.Fatal("pending frozen-input proof", err)
	}
	if err := tx.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestP13TowerDeferredOnePerOccupancyDrainPureP6AndRetry(t *testing.T) {
	f, _ := p13GuildSetup(t)
	if err := f.s.InitializeP13UnionFightTower(f.ctx); err != nil {
		t.Fatal(err)
	}
	tower, actor, captured, target := p13TowerPhysicalFixture(t)
	if err := db.GetIns().Create(tower).Error; err != nil {
		t.Fatal(err)
	}
	hit := p13TowerCaptureFixture(t, db.GetIns(), tower, target, 1, 1600, 2000)
	// Rebuild the light directory from durable rows as a cold worker would.
	target.occupancyShots = nil
	if err := p13UFTowerOccupancyShots(db.GetIns(), tower.MatchID, []*p13UFTowerTarget{target}); err != nil {
		t.Fatal(err)
	}
	repeated := p13TowerCaptureFixture(t, db.GetIns(), tower, target, 2, 6600, 7000)
	if repeated.Status != constUtil.P13UnionFightTowerHitCanceled || repeated.CoalescedInto != hit.ImmutableHash || len(repeated.Before) != 0 || len(repeated.After) != 0 {
		t.Fatal("late worker accumulated a second shot", repeated.Status)
	}
	if err := p13UFTowerPendingLife(db.GetIns(), actor.MatchID, actor.PlayerID, 0); err == nil {
		t.Fatal("next battle can bypass pending tower hit")
	}
	// This is a prospective scene-boundary fixture, not a fabricated P6 proof.
	p6After := proto.Clone(captured).(*pb.InstUFPlayer)
	p6After.State = proto.Int32(0)
	p6After.FightInfo.HpPos[0] = 80
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, _, err := p13UFTowerDrain(tx, actor, p6After, target.occupancyKind, target.occupancyID, strings.Repeat("f", 64), 5000, time.UnixMilli(8000))
		return err
	}); err == nil {
		t.Fatal("different P6 source drained occupancy")
	}
	var scene *pb.InstUFPlayer
	var frames []proto.Message
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		scene, frames, err = p13UFTowerDrain(tx, actor, p6After, target.occupancyKind, target.occupancyID, target.occupancyInputHash, 5000, time.UnixMilli(8000))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if scene.FightInfo.HpPos[0] != 44 || p6After.FightInfo.HpPos[0] != 80 || len(frames) != 2 {
		t.Fatal("P6 source overwritten or deferred missing", scene.FightInfo.HpPos[0], len(frames))
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		again, messages, err := p13UFTowerDrain(tx, actor, scene, target.occupancyKind, target.occupancyID, target.occupancyInputHash, 5000, time.UnixMilli(9000))
		if err == nil && (!proto.Equal(again, scene) || len(messages) != 0) {
			t.Fatal("retry double applied")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var saved db.P13UnionFightTowerHit
	if err := db.GetIns().First(&saved, "match_id=? AND tower_guid=? AND sequence=?", hit.MatchID, hit.TowerGUID, hit.Sequence).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != constUtil.P13UnionFightTowerHitApplied || saved.AppliedAt != 8000 || saved.HitAt != 1600 || saved.DueAt != 5000 || saved.ImmutableHash != hit.ImmutableHash || p13UFTowerHitVerify(&saved, tower) != nil {
		t.Fatal("deferred source/clock/immutable proof drift")
	}
	if err := p13UFTowerPendingLife(db.GetIns(), actor.MatchID, actor.PlayerID, 0); err != nil {
		t.Fatal("fully drained occupancy still busy", err)
	}
}

func TestP13TowerDeferredOldLifeDeadHeroAndLeftCancelWithoutRetarget(t *testing.T) {
	for _, kind := range []string{"life", "hero", "left"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := p13GuildSetup(t)
			tower, actor, frame, target := p13TowerPhysicalFixture(t)
			if err := db.GetIns().Create(tower).Error; err != nil {
				t.Fatal(err)
			}
			hit := p13TowerCaptureFixture(t, db.GetIns(), tower, target, 1, 1600, 2000)
			scene := proto.Clone(frame).(*pb.InstUFPlayer)
			scene.State = proto.Int32(0)
			reason := constUtil.P13UnionFightTowerCancelLife
			switch kind {
			case "life":
				scene.DeadNum = proto.Int32(1)
			case "hero":
				scene.FightInfo.HpPos[0] = 0
				reason = constUtil.P13UnionFightTowerCancelActor
			case "left":
				actor.LeftAt = 3000
				reason = constUtil.P13UnionFightTowerCancelLeft
			}
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				after, messages, err := p13UFTowerDrain(tx, actor, scene, target.occupancyKind, target.occupancyID, target.occupancyInputHash, 5000, time.UnixMilli(6000))
				if err == nil && (!proto.Equal(after, scene) || len(messages) != 0) {
					t.Fatal("canceled hit retargeted/healed/damaged")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			var saved db.P13UnionFightTowerHit
			if err := db.GetIns().First(&saved, "match_id=? AND tower_guid=? AND sequence=?", hit.MatchID, hit.TowerGUID, hit.Sequence).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Status != constUtil.P13UnionFightTowerHitCanceled || saved.CancelReason != reason || len(saved.Before) != 0 || len(saved.After) != 0 || saved.ImmutableHash != hit.ImmutableHash || p13UFTowerHitVerify(&saved, tower) != nil {
				t.Fatal("cancellation fact invalid", saved.CancelReason)
			}
			_ = f
		})
	}
}

func TestP13TowerDeferredDeathReviveStartsAtActualCommit(t *testing.T) {
	f, _ := p13GuildSetup(t)
	tower, actor, frame, target := p13TowerPhysicalFixture(t)
	frame.FightInfo.HpPos = []float64{20, 0, 0, 0}
	if err := db.GetIns().Create(tower).Error; err != nil {
		t.Fatal(err)
	}
	hit := p13TowerCaptureFixture(t, db.GetIns(), tower, target, 1, 1600, 2000)
	scene := proto.Clone(frame).(*pb.InstUFPlayer)
	scene.State = proto.Int32(0)
	var result *pb.InstUFPlayer
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		result, _, err = p13UFTowerDrain(tx, actor, scene, target.occupancyKind, target.occupancyID, target.occupancyInputHash, 5000, time.UnixMilli(10000))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.GetState() != 3 || result.GetDeadNum() != 1 || result.GetLeftTime() != 5 || result.GetKillNum() != scene.GetKillNum() {
		t.Fatal("environment death contract", result)
	}
	var saved db.P13UnionFightTowerHit
	if err := db.GetIns().First(&saved, "match_id=? AND tower_guid=? AND sequence=?", hit.MatchID, hit.TowerGUID, hit.Sequence).Error; err != nil {
		t.Fatal(err)
	}
	if saved.ReviveAt != 15000 || saved.DeadPlayerID != actor.PlayerID || saved.AppliedAt != 10000 || p13UFTowerHitVerify(&saved, tower) != nil {
		t.Fatal("late commit was incorrectly backdated", saved.ReviveAt)
	}
	_ = f
}

func TestP13TowerLaunchLifeAndHeroAreNotRetargetedAtImpact(t *testing.T) {
	for _, kind := range []string{"revived", "bound-hero-died", "entered-battle"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := p13GuildSetup(t)
			tower, _, launch, target := p13TowerPhysicalFixture(t)
			tower.Sequence = 1
			tower.PendingTargetGUID = target.guid
			tower.PendingTargetKind = "player"
			tower.PendingReleaseAt = 1600
			tower.NextFireAt = 1600
			originalSource := tower.SourceHash
			if err := p13UFTowerFreezeLaunch(tower, target); err != nil {
				t.Fatal(err)
			}
			tower.StateHash = p13UFTowerStateHash(tower)
			if !p13UFTowerValid(tower) || tower.SourceHash != originalSource || tower.PendingTargetLife != 0 || tower.PendingActorPose != 1 {
				t.Fatal("launch source was not frozen")
			}
			switch kind {
			case "revived":
				target.frame.DeadNum = proto.Int32(1)
			case "bound-hero-died":
				target.frame.FightInfo.HpPos[0] = 0
			case "entered-battle":
				target.frame.State = proto.Int32(1)
			}
			hit := &db.P13UnionFightTowerHit{MatchID: tower.MatchID, TowerGUID: tower.TowerGUID, Sequence: 1, TargetKind: "player", TargetGUID: target.guid, HitAt: 1600, DueAt: 1600, AppliedAt: 2000, TowerSourceHash: tower.SourceHash, Status: constUtil.P13UnionFightTowerHitApplied}
			if err := p13UFTowerCapturePhysical(db.GetIns(), hit, tower, target); err != nil {
				t.Fatal(err)
			}
			hit.HitHash = p13UFTowerHitHash(hit)
			if err := p13UFTowerHitVerify(hit, tower); err != nil {
				t.Fatal("impact source proof", err)
			}
			if hit.LifeOrdinal != 0 || hit.ActorGUID != "frozen-actor-a" || hit.ActorPose != 1 || target.frame.FightInfo.HpPos[1] != 50 {
				t.Fatal("old projectile switched life/hero")
			}
			if kind == "entered-battle" {
				if hit.Status != constUtil.P13UnionFightTowerHitPending || hit.LaunchOccupancyID != "" || hit.OccupancyID != target.occupancyID || hit.DueAt != 5000 {
					t.Fatal("idle launch did not defer into current battle")
				}
			} else if hit.Status != constUtil.P13UnionFightTowerHitCanceled || len(hit.Before) != 0 || len(hit.After) != 0 {
				t.Fatal("old life/dead hero projectile damaged current life")
			}
			_ = launch
			_ = f
		})
	}
}

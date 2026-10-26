package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// These are lifecycle fixtures only; no diagnostic terminal receives authority.
func p13GuardBuildingTowerFixture(t *testing.T) (*db.P13UnionFightGuardState, *db.P13UnionFightGuardBattleAttempt, *pb.InstUFGuard) {
	t.Helper()
	guard := &pb.InstUFGuard{Guid: proto.Int64(11), UFGuardId: proto.String("tower_guard_001"), Camp: proto.Int32(1), BindGuid: proto.Int64(22), State: proto.Int32(0), FightInfo: &pb.InstFightable{Guid: proto.Int64(11), Index: []int32{0}, HpPos: []float64{1000}, MaxHpPos: []float64{1000}, Anger: proto.Float64(0)}}
	initial, err := proto.Marshal(guard)
	if err != nil {
		t.Fatal(err)
	}
	current := proto.Clone(guard).(*pb.InstUFGuard)
	current.State = proto.Int32(2)
	current.FightInfo.HpPos[0] = 0
	current.FightInfo.Anger = proto.Float64(88)
	raw, err := proto.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	state := &db.P13UnionFightGuardState{MatchID: p6Digest([]byte(t.Name())), GuardGUID: 11, GuardID: "tower_guard_001", HeroID: "guard01_npc", Camp: 1, BindGUID: 22, BuildingKind: "tower", Generation: 2, InitialFrame: initial, InitialHash: p6Digest(initial), Frame: raw, FrameHash: p6Digest(raw), DefeatedAt: 5000, RecoverAt: 125000, BuildingEvent: "tower-defeated"}
	state.SourceHash = p13UFGuardStateHash(state)
	row := &db.P13UnionFightGuardBattleAttempt{ID: p6Digest([]byte(t.Name() + "attempt")), MatchID: state.MatchID, GuardGUID: 11, GuardGeneration: 1, Status: constUtil.P13UFGuardSettled, GuardDefeated: true, DueAt: 5000, SettledAt: 6000, BuildingEvent: "tower-defeated"}
	row.SourceHash = p13UFGuardBattleHash(row)
	return state, row, guard
}

func TestP13UFGuardBuildingTowerCrashBoundToSettledLedger(t *testing.T) {
	state, row, _ := p13GuardBuildingTowerFixture(t)
	service := &GameService{}
	frames, err := service.p13UFGuardBuildingApply(&gorm.DB{}, row, state, time.UnixMilli(6000))
	if err != nil || len(frames) != 1 || frames[0].(*pb.SCUFTowerCrash).GetTowerGuid() != 22 {
		t.Fatal("real tower crash protocol", frames, err)
	}
	if _, err = service.p13UFGuardBuildingApply(&gorm.DB{}, row, state, time.UnixMilli(4999)); err == nil {
		t.Fatal("early event accepted")
	}
	row.BuildingEvent = "base-defeated"
	if _, err = service.p13UFGuardBuildingApply(&gorm.DB{}, row, state, time.UnixMilli(6000)); err == nil {
		t.Fatal("altered event/hash accepted")
	}
	row.BuildingEvent = "tower-defeated"
	state.Frame[0] ^= 1
	if _, err = service.p13UFGuardBuildingApply(&gorm.DB{}, row, state, time.UnixMilli(6000)); err == nil {
		t.Fatal("corrupt frozen state accepted")
	}
}

func TestP13UFGuardBuildingTowerColdRecoveryExactInitialOnce(t *testing.T) {
	f, _ := p13GuildSetup(t)
	if err := db.GetIns().AutoMigrate(&db.P13UnionFightGuardState{}, &db.P13UnionFightField{}); err != nil {
		t.Fatal(err)
	}
	state, _, initial := p13GuardBuildingTowerFixture(t)
	buildingRaw, _ := proto.Marshal(&pb.SCUFBuildingRefresh{TowerInfos: []*pb.InstUFTower{{Guid: proto.Int64(22), Guard: initial, State: proto.Int32(0)}}})
	snapshotRaw, _ := json.Marshal(p13UFFieldSnapshot{Buildings: buildingRaw})
	if err := db.GetIns().Create(&db.P13UnionFightField{MatchID: state.MatchID, Snapshot: snapshotRaw, SnapshotHash: p6Digest(snapshotRaw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(state).Error; err != nil {
		t.Fatal(err)
	}
	match := &db.P13UnionFightMatch{ID: state.MatchID, EndAt: 1000000}
	run := func(at int64) []proto.Message {
		t.Helper()
		var frames []proto.Message
		err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			var err error
			frames, err = (&GameService{}).p13UFGuardBuildingAdvance(tx, match, time.UnixMilli(at))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return frames
	}
	if len(run(124999)) != 0 {
		t.Fatal("tower recovered early")
	}
	frames := run(125000)
	if len(frames) != 1 || !proto.Equal(frames[0].(*pb.SCUFBuildingRefresh).TowerInfos[0].Guard, initial) {
		t.Fatal("initial HP/AP/frame not exactly restored")
	}
	var saved db.P13UnionFightGuardState
	if err := db.GetIns().First(&saved, "match_id=? AND guard_guid=?", state.MatchID, state.GuardGUID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Generation != 3 || saved.RecoverAt != 0 || saved.BuildingEvent != "" || saved.SourceHash != p13UFGuardStateHash(&saved) {
		t.Fatal("recovery ledger not closed", saved.Generation)
	}
	if len(run(130000)) != 0 {
		t.Fatal("recovery replayed")
	}
	_ = f
}

func TestP13UFGuardBuildingBaseRequiresAuthenticatedEnemyGate(t *testing.T) {
	_, _ = p13GuildSetup(t)
	if err := db.GetIns().AutoMigrate(&db.P13UnionFightGateState{}, &db.P13UnionFightField{}); err != nil {
		t.Fatal(err)
	}
	scope := p6Digest([]byte(t.Name()))
	// Explicit legacy match is needed by authenticated base admission.
	if err := db.GetIns().Create(&db.P13UnionFightMatch{ID: scope, StartAt: time.Now().Add(-time.Hour).UnixMilli(), EndAt: time.Now().Add(time.Hour).UnixMilli(), LeftPreview: []byte{}, RightPreview: []byte{}, Roster: []byte{}}).Error; err != nil {
		t.Fatal(err)
	}
	actor := &db.P13UnionFightFieldPlayer{MatchID: scope, PlayerID: 9, Camp: 0}
	guard := &pb.InstUFGuard{Guid: proto.Int64(111), BindGuid: proto.Int64(222), Camp: proto.Int32(1)}
	raw, _ := proto.Marshal(&pb.SCUFBuildingRefresh{BaseCampInfos: []*pb.InstUFBaseCamp{{Guid: proto.Int64(222), Guard: guard}}})
	snapshot, _ := json.Marshal(p13UFFieldSnapshot{Buildings: raw})
	if err := db.GetIns().Create(&db.P13UnionFightField{MatchID: scope, Snapshot: snapshot, SnapshotHash: p6Digest(snapshot)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p13UFGuardBuildingBaseCheck(db.GetIns(), actor, guard); err == nil {
		t.Fatal("missing destroyed gate admitted")
	}
	gate := &pb.InstUFGate{Guid: proto.Int64(p13UFFieldGUID(scope, "gate", "1")), Id: proto.Int32(1), Camp: proto.Int32(1), Level: proto.Int32(1), MaxHp: proto.Float64(1000), Hp: proto.Float64(1000)}
	row := db.P13UnionFightGateState{MatchID: scope, GUID: gate.GetGuid(), GateID: 1, Camp: 1, InitialHash: p6Digest([]byte("explicit gate fixture")), Source: "explicit lifecycle test"}
	if err := p13UFGateSave(db.GetIns(), &row, gate); err != nil {
		t.Fatal(err)
	}
	if _, err := p13UFGuardBuildingBaseCheck(db.GetIns(), actor, guard); err == nil {
		t.Fatal("intact gate admitted")
	}
	gate.Hp = proto.Float64(0)
	if err := p13UFGateSave(db.GetIns(), &row, gate); err != nil {
		t.Fatal(err)
	}
	proof, err := p13UFGuardBuildingBaseCheck(db.GetIns(), actor, guard)
	if err != nil || len(proof) != 64 {
		t.Fatal("authenticated destroyed enemy gate refused", err)
	}
	actor.Camp = 1
	if _, err := p13UFGuardBuildingBaseCheck(db.GetIns(), actor, guard); err == nil {
		t.Fatal("friendly base admitted")
	}
	actor.Camp = 0
	if err := db.GetIns().Model(&row).Update("frame_hash", "corrupt").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p13UFGuardBuildingBaseCheck(db.GetIns(), actor, guard); err == nil {
		t.Fatal("corrupt gate granted proof")
	}
}

func TestP13UFGuardBuildingFactoryStopsAndColdResumesWithoutCatchup(t *testing.T) {
	f, _, first, _, match := p13UFFieldSetup(t)
	if err := f.s.InitializeP13UnionFightGuardBattle(f.ctx); err != nil {
		t.Fatal(err)
	}
	policy := &db.P13UnionFightFieldPolicy{PreparationMillis: 300000, LeftRevivePointID: 1, RightRevivePointID: 3, InitialGuardHP: 1000, InitialGateHP: 10000, MoveTolerance: 3, Source: "explicit lifecycle fixture"}
	var snapshot *p13UFFieldSnapshot
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = f.s.p13UFFieldBuild(tx, match, policy, time.UnixMilli(first-1000))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var buildings pb.SCUFBuildingRefresh
	if proto.Unmarshal(snapshot.Buildings, &buildings) != nil {
		t.Fatal("real frozen factory frame")
	}
	var frame *pb.InstUFCariotFactory
	for _, candidate := range buildings.FactoryInfos {
		if candidate.GetGuard().GetFightInfo() != nil {
			// Explicit already-open field-factory lifecycle prerequisite.
			candidate.IsOpen = proto.Int32(1)
			candidate.State = proto.Int32(0)
			frame = candidate
			break
		}
	}
	if frame == nil {
		t.Fatal("original guarded field factory missing")
	}
	var factory db.P13UnionFightFactoryState
	if err := db.GetIns().First(&factory, "match_id=? AND guid=?", match.ID, frame.GetGuid()).Error; err != nil {
		t.Fatal(err)
	}
	// Explicit producer resource fixture, keeping original factory ID/camp/frame.
	factory.ChariotHP = 1000
	factory.SourceHash = p13UFFactoryDigest(&factory)
	factory.StartedAt = first
	factory.AnimationAt = first + 111330
	factory.DueAt = first + 120000
	if err := p13UFFactorySave(db.GetIns(), &factory, frame); err != nil {
		t.Fatal(err)
	}
	initial, _ := proto.Marshal(frame.Guard)
	dead := proto.Clone(frame.Guard).(*pb.InstUFGuard)
	dead.State = proto.Int32(2)
	dead.FightInfo.HpPos[0] = 0
	deadRaw, _ := proto.Marshal(dead)
	state := &db.P13UnionFightGuardState{MatchID: match.ID, GuardGUID: dead.GetGuid(), GuardID: dead.GetUFGuardId(), Camp: int64(dead.GetCamp()), BindGUID: frame.GetGuid(), BuildingKind: "factory", Generation: 2, InitialFrame: initial, InitialHash: p6Digest(initial), Frame: deadRaw, FrameHash: p6Digest(deadRaw), DefeatedAt: first + 5000, RecoverAt: first + 305000, BuildingEvent: "factory-defeated"}
	state.SourceHash = p13UFGuardStateHash(state)
	if err := db.GetIns().Create(state).Error; err != nil {
		t.Fatal(err)
	}
	row := &db.P13UnionFightGuardBattleAttempt{MatchID: match.ID, GuardGUID: state.GuardGUID, Status: constUtil.P13UFGuardSettled, GuardDefeated: true, BuildingEvent: state.BuildingEvent, DueAt: state.DefeatedAt, SettledAt: first + 6000}
	row.SourceHash = p13UFGuardBattleHash(row)
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, err := f.s.p13UFGuardBuildingApply(tx, row, state, time.UnixMilli(first+6000))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&factory, "match_id=? AND guid=?", match.ID, frame.GetGuid()).Error; err != nil {
		t.Fatal(err)
	}
	paused, err := p13UFFactoryFrame(&factory)
	if err != nil || paused.GetState() != 2 || factory.DueAt != 0 || factory.AnimationAt != 0 || factory.CompletedAt != 0 {
		t.Fatal("first car was not canceled", err)
	}
	// Test a long match explicitly, without changing the persisted match EndAt.
	longMatch := *match
	longMatch.EndAt = first + 900000
	at := first + 400000
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, err := (&GameService{}).p13UFGuardBuildingAdvance(tx, &longMatch, time.UnixMilli(at))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&factory, "match_id=? AND guid=?", match.ID, frame.GetGuid()).Error; err != nil {
		t.Fatal(err)
	}
	restored, err := p13UFFactoryFrame(&factory)
	if err != nil || restored.GetState() != 0 || !proto.Equal(restored.Guard, frame.Guard) || factory.StartedAt != at || factory.DueAt != at+120000 {
		t.Fatal("cold factory recovery caught up old production or changed guard", factory.StartedAt, factory.DueAt, err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightCarGeneration{}).Where("match_id=? AND factory_guid=?", match.ID, frame.GetGuid()).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("recovery manufactured a car", count, err)
	}
}

func TestP13UFGuardMirrorBaseAdmissionBoundaryAndLegacyProof(t *testing.T) {
	gate := &pb.InstUFGate{Hp: proto.Float64(0)}
	for _, tc := range []struct {
		camp, id, at int64
		want         bool
	}{{0, 1, 1499, false}, {0, 1, 1500, true}, {0, 2, 1500, true}, {0, 3, 1500, false}, {1, 5, 1500, true}, {1, 6, 1500, true}, {1, 7, 1500, false}, {0, 1, 2000, false}} {
		if got := p13UFGuardBaseGateAllowed(constUtil.P13UFGuardMirrorRule, tc.camp, tc.id, 1000, 2000, tc.at, gate); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	gate.Hp = proto.Float64(1)
	if p13UFGuardBaseGateAllowed(constUtil.P13UFGuardMirrorRule, 0, 1, 1000, 2000, 1500, gate) {
		t.Fatal("living gate opened the base")
	}
	gate.Hp = proto.Float64(0)
	if !p13UFGuardBaseGateAllowed("", 0, 3, 1000, 2000, 1001, gate) {
		t.Fatal("legacy admission reinterpreted")
	}
	row := &db.P13UnionFightGateState{GUID: 42, SourceHash: p6Digest([]byte("gate"))}
	raw, _ := json.Marshal(struct {
		Match               string
		Player, Guard, Gate int64
		GateHash            string
	}{"match", 7, 8, row.GUID, row.SourceHash})
	old := p13UFGuardBaseGateProof("match", 7, 8, row, constUtil.P13UFGuardLocalRule, 1000, 2000, 1500)
	if old != p6Digest(raw) {
		t.Fatal("legacy proof bytes changed")
	}
	fresh := p13UFGuardBaseGateProof("match", 7, 8, row, constUtil.P13UFGuardMirrorRule, 1000, 2000, 1500)
	if fresh == old || fresh == p13UFGuardBaseGateProof("match", 7, 8, row, constUtil.P13UFGuardMirrorRule, 1000, 2000, 1501) {
		t.Fatal("new proof did not bind rule/capture time")
	}
}

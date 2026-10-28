package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOMonsterRecordedEndRequiresSameRequest(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	_ = p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldReceipt{}); err != nil {
		t.Fatal(err)
	}
	fight := "0123456789abcdef01234567"
	request := &pb.CSSidePveFightEnd{FightId: proto.String(fight), Result: proto.Int32(1)}
	hash, err := ggoMonsterEndHash(request)
	if err != nil {
		t.Fatal(err)
	}
	native := &pb.SCSidePveFightEnd{Code: proto.Int32(1), FightResult: proto.Int32(1), Getn: &pb.ResultObject{}}
	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	record := model.GGOFieldMonsterReceipt{FightID: fight, Status: "settled", DeliveryStatus: "pending_delivery", InputHash: "frozen-test-digest", EndRequestHash: hash, NativeEnd: wire}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		return ggoSaveReceipt(tx, 10001, ggoSystemReceiptID("monster-result", fight), constUtil.GGOFieldOpMonsterResult, hash, "test", &fight, [6]int64{}, 0, time.Now().UnixMilli(), record)
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		out, found, err := ggoMonsterRecordedEnd(db.GetIns(), 10001, request)
		if err != nil || !found || !proto.Equal(out, native) {
			t.Fatal("exact original response not recovered", found, err)
		}
	}
	changed := proto.Clone(request).(*pb.CSSidePveFightEnd)
	changed.Result = proto.Int32(0)
	if _, found, err := ggoMonsterRecordedEnd(db.GetIns(), 10001, changed); !found || err == nil {
		t.Fatal("different end payload accepted", found, err)
	}
	if _, found, err := ggoMonsterRecordedEnd(db.GetIns(), 10002, request); found || err != nil {
		t.Fatal("another player's receipt leaked", found, err)
	}
	var count int64
	if err := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND operation=?", 10001, constUtil.GGOFieldOpMonsterResult).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("replay duplicated result", count, err)
	}
}

func TestGGOMonsterTerminalUsesFinalPose(t *testing.T) {
	first := p6Actor("hero_047", "first", "047_1", 3, 100)
	second := p6Actor("hero_047", "second", "047_1", 7, 200)
	party := &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{first, second}, Inst: &pb.BattlePlayerInst{}}
	terminal := &model.CombatFinalHPOutcome{Outcome: &model.CombatOutcome{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Inst: &pb.BattlePlayerInst{Anger: proto.Float64(10)}}}, FinalActors: []model.CombatFinalActorHP{
		{Camp: 0, GUID: "first", HeroID: first.GetId(), Pose: 3, RuntimePose: 7, MaxHP: 100, CurrentHP: 50},
		{Camp: 0, GUID: "second", HeroID: second.GetId(), Pose: 7, RuntimePose: 3, MaxHP: 200, CurrentHP: 100},
	}}
	reported := proto.Clone(party).(*pb.BattlePlayer)
	reported.Formation[0].Pose = proto.Int32(7)
	reported.Formation[0].Inst.CurHp = proto.Float64(50)
	reported.Formation[1].Pose = proto.Int32(3)
	reported.Formation[1].Inst.CurHp = proto.Float64(100)
	if err := ggoMonsterReportedHP(42, reported, &model.FrozenBattleInput{Attacker: party}, terminal); err != nil {
		t.Fatal("valid original final positions rejected", err)
	}
	side := &pb.PlayerSide{PlayerId: proto.Int64(42)}
	if err := ggoMonsterApplyTerminal(party, side, terminal); err != nil {
		t.Fatal(err)
	}
	if len(side.BattlePosHps) != 2 || side.BattlePosHps[0].GetBattlePos() != 7 || side.BattlePosHps[1].GetBattlePos() != 3 || side.GetBattlePos3() != .5 || side.GetBattlePos7() != .5 {
		t.Fatal("final pose HP projection incorrect", side)
	}
	reported.Formation[0].Pose = proto.Int32(3)
	if ggoMonsterReportedHP(42, reported, &model.FrozenBattleInput{Attacker: party}, terminal) == nil {
		t.Fatal("initial pose accepted after swap")
	}
}

func TestGGOMonsterReservationSurvivesElapsedTime(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	_ = p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldLine{}); err != nil {
		t.Fatal(err)
	}
	cfg := ggoLineTestConfiguration(t)
	now := time.Now().UnixMilli()
	var generation int64
	var original string
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, 87501, 1, cfg, "test", now)
		if err != nil {
			return err
		}
		node := &state.Monsters[0]
		node.ReservedBy, node.FightID, node.ReservedUntilMS = 10001, "0123456789abcdef01234567", 0
		generation, original = node.Generation, node.UUID
		return ggoLineSave(tx, row, state, now)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, state, _, err := ggoLoadLine(tx, 87501, 1, cfg, "test", now+3600000)
		if err != nil {
			return err
		}
		node := state.Monsters[0]
		if node.ReservedBy != 10001 || node.FightID == "" || node.Generation != generation || node.UUID != original {
			t.Fatal("elapsed time replaced in-flight battle", node)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

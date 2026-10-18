package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p7AwakeSetup(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7Setup(t)
	if err := f.s.LoadP7AwakeTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP7Awake(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P7AwakePolicy{}, &db.P7AwakeStageRule{}, &db.P7AwakeState{}, &db.P7AwakeRun{}, &db.P7AwakeExploration{}, &db.P7AwakeFight{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP7Awake(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f
}
func p7AwakeDiagnosticRule(t *testing.T) db.P7AwakeStageRule {
	t.Helper()
	content := p7AwakeContent{Source: "隔离明确节点样本，仅原模型和安全点，不宣称恢复原难度地图", Boxes: []*pb.AwakeCopyBox{{BoxId: proto.String("box1"), Guid: proto.String("box-1"), Pos: proto.String("36,1,56"), IsOpen: proto.Int32(0), Angle: proto.Float64(0)}}, Monsters: []*pb.AwakeCopyMonster{{MonId: proto.String("mon_001"), Guid: proto.String("mon-1"), Pos: proto.String("37,1,56"), HpPercent: proto.Float64(1), Angle: proto.Float64(0), MonsterType: proto.Int32(1)}}}
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	return db.P7AwakeStageRule{CopyID: "101", ChapterID: "1", ContentRestored: true, FightPower: 100, Content: raw, Source: content.Source}
}
func TestP7AwakeOriginalWeekdayCatalogueBudgetAndGate(t *testing.T) {
	f := p7AwakeSetup(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	r, err := f.s.p7AwakeCatalogue(f.ctx, f.id, now)
	if err != nil {
		t.Fatal(err)
	}
	catalog := r[0].(*pb.SCAwakeCopyData)
	if catalog.GetLastNum() != 2 || len(catalog.GetCopys()) != 5 {
		t.Fatal("original five chapters/two attempts")
	}
	for _, chapter := range catalog.GetCopys() {
		expected := int32(0)
		if chapter.GetChapterId() == "1" {
			expected = 1
		}
		if chapter.GetIsOpen() != expected || len(chapter.GetSmallCopys()) != 0 {
			t.Fatal("invented tasks/calendar", chapter)
		}
	}
	if err = db.GetIns().Model(&db.P7AwakeState{}).Where("player_id=?", f.id).Update("used", 1).Error; err != nil {
		t.Fatal(err)
	}
	r, err = f.s.p7AwakeCatalogue(f.ctx, f.id, now)
	if err != nil || r[0].(*pb.SCAwakeCopyData).GetLastNum() != 1 {
		t.Fatal("stored daily budget", err)
	}
	r, err = f.s.p7AwakeCatalogue(f.ctx, f.id, now.AddDate(0, 0, 6))
	if err != nil {
		t.Fatal(err)
	}
	for _, chapter := range r[0].(*pb.SCAwakeCopyData).GetCopys() {
		if chapter.GetIsOpen() != 1 {
			t.Fatal("original Sunday all open")
		}
	}
	if r[0].(*pb.SCAwakeCopyData).GetLastNum() != 2 {
		t.Fatal("daily reset")
	}
	if err = db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	for _, request := range []proto.Message{&pb.CSEnterAwakeCopy{CopyId: proto.String("101")}, &pb.CSAwakeCopyOpenBox{BoxGuid: proto.String("box-1")}, &pb.CSAwakeCopySettle{}, &pb.CSAwakeRevice{}, &pb.CSAwakeCopyFightEnd{IsWinClient: proto.Int32(1)}} {
		if _, handled, err := f.s.DispatchP7Awake(f.ctx, f.id, request, 1); !handled || err == nil {
			t.Fatal("SQL/native-incomplete gate", request)
		}
	}
	after, ag := f.profile(t)
	if after.GetDiamond() != p.GetDiamond() || after.GetEnergy() != p.GetEnergy() || simpleOwned(after, ag, "item_320") != simpleOwned(p, g, "item_320") || p1Count(t, &db.P7AwakeRun{}, "player_id=?", f.id) != 0 {
		t.Fatal("closed mutation changed save")
	}
}
func TestP7AwakeRealActorAndOriginalFrameFrozenAfterRestart(t *testing.T) {
	f := p7AwakeSetup(t)
	rule := p7AwakeDiagnosticRule(t)
	if err := db.GetIns().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	before, bg := f.profile(t)
	now := time.Now()
	id, reply, err := f.s.PrepareP7AwakeDiagnostic(f.ctx, f.id, rule.CopyID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.GetBoxes()) != 1 || len(reply.GetMonsters()) != 1 || reply.GetRefreshData().GetEndTime()-now.UnixMilli() != 30*60000 || reply.GetRefreshData().GetLeftReviveCount() != 3 || reply.GetRefreshData().GetReviveCost() != 0 {
		t.Fatal("original node/frame/rules")
	}
	var row db.P7AwakeRun
	if err = db.GetIns().Where("id=?", id).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var frozen p7AwakeFrozen
	if json.Unmarshal(row.Freeze, &frozen) != nil || frozen.Scene != "map_nightforestfield_001" || frozen.StaminaCost != 0 || frozen.ReviveCost != 100 {
		t.Fatal("original frozen scene/cost")
	}
	actor := &pb.BattlePlayer{}
	if proto.Unmarshal(row.Actor, actor) != nil || actor.GetPlayerId() != f.id || len(actor.GetFormation()) == 0 || actor.GetFormation()[0].GetSkillOrder() != "047_1,047_2,047_3" {
		t.Fatal("actual complete actor snapshot", actor)
	}
	if err = db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", rule.CopyID).Update("content", []byte("bad live config")).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = f.s.LoadP7AwakeTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeP7Awake(f.ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.ReadP7AwakeDiagnostic(f.ctx, f.id, id)
	if err != nil || !proto.Equal(reply, restored) {
		t.Fatal("restart replaced frozen node/frame", err)
	}
	after, ag := f.profile(t)
	if after.GetEnergy() != before.GetEnergy() || after.GetDiamond() != before.GetDiamond() || simpleOwned(after, ag, "item_320") != simpleOwned(before, bg, "item_320") {
		t.Fatal("diagnostic changed resources")
	}
	if p1Count(t, &db.P7AwakeState{}, "player_id=?", f.id) != 0 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "awake_complete") != 0 {
		t.Fatal("diagnostic manufactured progress")
	}
	if _, err = f.s.ReadP7AwakeDiagnostic(f.ctx, f.id+1, id); err == nil {
		t.Fatal("foreign run")
	}
	if err = db.GetIns().Model(&db.P7AwakeRun{}).Where("id=?", id).Update("actor", []byte("tampered")).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReadP7AwakeDiagnostic(f.ctx, f.id, id); err == nil {
		t.Fatal("actor tamper")
	}
}
func TestP7AwakeRejectsInventedTaskMalformedPositionAndMissingSource(t *testing.T) {
	f := p7AwakeSetup(t)
	base := p7AwakeDiagnosticRule(t)
	for _, mutate := range []func(*db.P7AwakeStageRule){
		func(r *db.P7AwakeStageRule) { r.CopyID = "invented-difficulty" },
		func(r *db.P7AwakeStageRule) { r.ContentRestored = false },
		func(r *db.P7AwakeStageRule) { r.Source = "" },
		func(r *db.P7AwakeStageRule) {
			var c p7AwakeContent
			json.Unmarshal(r.Content, &c)
			c.Boxes[0].Pos = proto.String("NaN,1,2")
			r.Content, _ = json.Marshal(c)
		},
		func(r *db.P7AwakeStageRule) {
			var c p7AwakeContent
			json.Unmarshal(r.Content, &c)
			c.Monsters[0].Guid = c.Boxes[0].Guid
			r.Content, _ = json.Marshal(c)
		},
		func(r *db.P7AwakeStageRule) {
			var c p7AwakeContent
			json.Unmarshal(r.Content, &c)
			c.Boxes[0].BoxId = proto.String("missing")
			r.Content, _ = json.Marshal(c)
		},
	} {
		row := base
		mutate(&row)
		if _, err := f.s.p7AwakeFreeze(row); err == nil {
			t.Fatal("unknown/malformed map imported", row.CopyID)
		}
	}
	if _, err := p7AwakeOpenDay("1,8", time.Now()); err == nil {
		t.Fatal("invalid weekday")
	}
}

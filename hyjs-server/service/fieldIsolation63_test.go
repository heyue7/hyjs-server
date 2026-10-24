package service

import (
	"encoding/json"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This SQLite regression denies every shared-boss query while exercising actual
// small-monster checks and frozen starts. MySQL lock wait timing remains an
// integration check; reaching no shared query removes that lock dependency.
func TestField63SmallMonsterDoesNotReadSharedBoss(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(40); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldScene(f.ctx, f.id, false); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		return f.s.refreshSharedFieldBosses55(tx, g)
	}); err != nil {
		t.Fatal(err)
	}
	var shared []db.SharedFieldBoss55
	if err := db.GetIns().Order("server_id,scene,pos_id").Find(&shared).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(shared)
	_, g := fieldBoss33State(t, f.id)
	uuid := g.Field.Monsters[0].GetUuid()
	hp := g.Field.Side.GetCurHp()
	hits := 0
	deny := func(tx *gorm.DB) {
		if tx.Statement.Table == "shared_field_boss55" {
			hits++
			tx.AddError(errors.New("shared boss row is unavailable"))
		}
	}
	if err := db.GetIns().Callback().Query().Before("gorm:query").Register("field63_deny_shared", deny); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove("field63_deny_shared")
	if _, err := f.s.fieldMonsterCheck(f.ctx, f.id, &pb.CSSideMonsterCheck{Uuid: proto.String(uuid)}); err != nil {
		t.Fatal(err)
	}
	start, err := f.s.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(uuid)})
	if err != nil || start[0].(*pb.SCSidePveFightStart).GetCode() != 1 {
		t.Fatal("small-monster start depended on shared boss", err)
	}
	retry, err := f.s.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(uuid)})
	if err != nil || !proto.Equal(start[0], retry[0]) {
		t.Fatal("frozen retry changed", err)
	}
	_, g = fieldBoss33State(t, f.id)
	if g.Field.Side.GetCurHp() != hp || g.Field.Fight == nil || g.Field.Fight.Settled {
		t.Fatal("small-monster start changed HP or lost frozen fight")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Field.Fight.BossStart = &pb.SCSidePveBossFightStart{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(uuid)}); err == nil {
		t.Fatal("pending boss fight was bypassed")
	}
	if hits != 0 {
		t.Fatalf("small-monster path attempted %d shared boss reads", hits)
	}
	if err := db.GetIns().Callback().Query().Remove("field63_deny_shared"); err != nil {
		t.Fatal(err)
	}
	shared = nil
	if err := db.GetIns().Order("server_id,scene,pos_id").Find(&shared).Error; err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(shared)
	if string(before) != string(after) {
		t.Fatal("shared boss generation/assets changed")
	}
}

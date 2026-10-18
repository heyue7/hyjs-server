package service

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP12FieldPVPIntegrationSelectiveControlAndQueryRecovery(t *testing.T) {
	f, _, _, sessions := p12FieldPVPSetup(t)
	f.s.P12FieldPVPReady = true
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	now := time.Now()
	state := db.P12FieldPVPState{PlayerID: f.id, BusyUntil: now.Add(time.Minute).UnixMilli(), Source: constUtil.P12FieldPVPSource}
	if err := db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	// This unit callback represents an unrelated profile transaction. It grants
	// no resources and supplies no fight outcome; broad save guards must not run.
	ran := false
	if err := updateProfileWithTx(ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error { ran = true; return nil }); err != nil || !ran {
		t.Fatal("busy blocked unrelated profile transaction", err)
	}
	ran = false
	if err := f.s.p3Profile(ctx, f.id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error { ran = true; return nil }); err != nil || !ran {
		t.Fatal("busy blocked unrelated p3 callback", err)
	}
	if _, _, err := f.s.resourceProfile(ctx, f.id); err != nil {
		t.Fatal("busy blocked resource query", err)
	}
	if paused, err := f.s.pauseP12FieldPVPSurvival(db.GetIns(), f.id, now); err != nil || !paused {
		t.Fatal("passive survival was not paused", err)
	}
	move := &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(69), Y: proto.Float32(15), Z: proto.Float32(84)}}
	moveContext := context.WithValue(ctx, constUtil.GameRequestContextKey, move)
	if _, err := f.s.dispatch(moveContext, &model.GamePacket{PlayerId: f.id, ZoneId: int32(f.s.OriginalServerID()), Session: sessions[f.id], Body: move}, ""); err == nil {
		t.Fatal("busy original movement escape")
	}
	if _, err := f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err == nil {
		t.Fatal("busy scene escape")
	}
	if _, err := f.s.fieldScene(ctx, f.id, false); err == nil {
		t.Fatal("busy current-position handshake")
	}
	if _, err := f.s.fieldScene(ctx, f.id, true); err != nil {
		t.Fatal("busy login recovery query rejected", err)
	}
	if err := f.s.ensureNoActiveDailyCopy(db.GetIns(), f.id, now); err == nil {
		t.Fatal("busy cross-mode battle allowed")
	}
	if _, err := f.s.p8LifeMutation(ctx, f.id, constUtil.P8ResurrectOperation, 0, &pb.CSResurrection{}, &pb.SCResurrenttion{}, func(*gorm.DB, *pb.Player, *model.GameState, int64) error {
		t.Error("busy revive action ran")
		return nil
	}); err == nil {
		t.Fatal("busy revive allowed")
	}
	if _, err := f.s.p8GatherMutation(ctx, f.id, constUtil.P8GatherStartOperation, 0, &pb.CSFieldResourceStartGather{}, &pb.SCFieldResourceStartGather{}, func(*gorm.DB, *pb.Player, *model.GameState, int64) error {
		t.Error("busy gather action ran")
		return nil
	}); err == nil {
		t.Fatal("busy gather allowed")
	}
	// A persisted judicial sentence is an explicit boundary fixture. The lazy
	// recovery must commit on a read even when resource clocks have no changes.
	state.BusyUntil = 0
	state.JailUntil = now.Add(time.Hour).UnixMilli()
	state.Kills = 6
	if err := db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.RedeemTime = proto.Int64(state.JailUntil)
		g.Field.Side.Kills = proto.Int32(6)
		g.Field.Side.State = proto.Int32(5)
		g.Field.Side.CurHp = proto.Float64(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, g, err := f.s.resourceProfile(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if g.Position.GetScene() != "town_main" || !p12FieldPVPPrisonPoint(g.Position.GetCurrent()) || g.Field.Side.GetKills() != 6 || g.Field.Side.GetCurHp() <= 0 {
		t.Fatal("read did not recover original prison state")
	}
	var row db.Profile
	if err = db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, cold, err := decodeProfile(&row)
	if err != nil || cold.Position.GetScene() != "town_main" {
		t.Fatal("query prison recovery not persisted", err)
	}
	if _, err = f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_002")}); err == nil {
		t.Fatal("jailed field escape")
	}
	response, err := f.s.fieldScene(ctx, f.id, false)
	if err != nil {
		t.Fatal("original prison CurrentPosition handshake refused", err)
	}
	self := response[0].(*pb.SCCurrentPosition).GetSelf()
	if self.GetInfo().GetPlayerSide().GetKills() != 6 {
		t.Fatal("Town CurrentPosition washed original jailFlag")
	}
	frame, err := f.s.p10SceneMovePlayer(p, g)
	if err != nil || frame.GetInfo().GetPlayerSide().GetKills() != 6 {
		t.Fatal("Town presence washed original jailFlag", err)
	}
	if _, err = f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("town_main"), Point: g.Position.Current}); err != nil {
		t.Fatal("original scripted prison transfer refused", err)
	}
	if _, err = f.s.p12FieldPVPRedeemAt(ctx, f.id, sessions[f.id], &pb.CSPlayerRedeem{Type: proto.Int32(0)}, 0, now.Add(time.Hour)); err != nil {
		t.Fatal("natural release boundary", err)
	}
	f.s.P12FieldPVPReady = false
	if paused, err := f.s.pauseP12FieldPVPSurvival(db.GetIns(), f.id, now); err != nil || paused {
		t.Fatal("uninitialized legacy service behavior changed", err)
	}
}

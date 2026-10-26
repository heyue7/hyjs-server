package service

import (
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// This regression enters a genuinely matched roster and sends CSMove through
// the same authenticated top-level dispatcher used by the original TCP client.
func TestP13UFMoveMainDispatchKeepsFieldProfileAndFrameConsistent(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated real main-dispatch movement fixture; no combat or result authority"}).Error; err != nil {
		t.Fatal(err)
	}
	id := ids[0]
	var profile db.Profile
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.s.RegisterP10Client(f.ctx, id, profile.Session, func(proto.Message) error { return nil }))
	if _, err := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	for _, message := range f.s.Dispatch(f.ctx, scene49Packet(t, f, id, 30024, &pb.CSCurrentPosition{}), "127.0.0.1") {
		if failure, ok := message.(*pb.ErrorMessage); ok {
			t.Fatal("actual Current handshake", failure.GetMsg())
		}
	}
	if _, err := f.s.SyncP13UnionFightField(f.ctx, id, false); err != nil {
		t.Fatal(err)
	}
	read := func() (db.P13UnionFightFieldPlayer, *pb.PointNew) {
		t.Helper()
		var actor db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		var saved db.Profile
		if err := db.GetIns().First(&saved, id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&saved)
		if err != nil {
			t.Fatal(err)
		}
		return actor, proto.Clone(game.Position.Current).(*pb.PointNew)
	}
	actor, _ := read()
	snapshot, err := p13UFFieldStoredProjection(db.GetIns(), match.ID)
	if err != nil {
		t.Fatal(err)
	}
	point := &pb.PointNew{X: proto.Float32(actor.X + 0.25), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)}
	if point.GetX() > snapshot.Maximum[0] {
		point.X = proto.Float32(actor.X - 0.25)
	}
	clock.Store(first + 1000)
	var actorWrites atomic.Int64
	callback := "guild-move-committed-presence"
	if err := db.GetIns().Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p13_union_fight_field_players" {
			actorWrites.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Update().Remove(callback) })
	messages := f.s.Dispatch(f.ctx, scene49Packet(t, f, id, 30001, &pb.CSMove{PointNew: point}), "127.0.0.1")
	var movement *pb.SCUFElementMove
	for _, message := range messages {
		if failure, ok := message.(*pb.ErrorMessage); ok {
			t.Fatal("valid authenticated war move", failure.GetMsg())
		}
		if frame, ok := message.(*pb.SCUFElementMove); ok {
			movement = frame
		}
	}
	if movement == nil || len(movement.Guid) != 1 || movement.Guid[0] != id || len(movement.Pos) != 1 || !proto.Equal(movement.Pos[0], point) {
		t.Fatal("main dispatch omitted original war movement frame", messages)
	}
	if writes := actorWrites.Load(); writes != 1 {
		t.Fatal("a committed move repeated the field presence update before its socket reply", writes)
	}
	after, saved := read()
	if after.X != point.GetX() || after.Y != point.GetY() || after.Z != point.GetZ() || !proto.Equal(saved, point) {
		t.Fatal("main dispatch split field/profile positions", after.X, after.Y, after.Z, saved)
	}
	bad := proto.Clone(point).(*pb.PointNew)
	bad.X = proto.Float32(float32(float64(snapshot.Maximum[0]) + snapshot.MoveTolerance + 1000))
	rejected := f.s.Dispatch(f.ctx, scene49Packet(t, f, id, 30001, &pb.CSMove{PointNew: bad}), "127.0.0.1")
	refused := false
	for _, message := range rejected {
		if _, ok := message.(*pb.ErrorMessage); ok {
			refused = true
		}
		if _, ok := message.(*pb.SCUFElementMove); ok {
			t.Fatal("rejected move published a movement frame")
		}
	}
	if !refused {
		t.Fatal("out-of-range main-dispatch move accepted", rejected)
	}
	unchanged, position := read()
	if unchanged.X != after.X || unchanged.Y != after.Y || unchanged.Z != after.Z || unchanged.FrameHash != after.FrameHash || unchanged.UpdatedAt != after.UpdatedAt || unchanged.MovementDebt != after.MovementDebt || !proto.Equal(position, saved) {
		t.Fatal("rejected move changed actor/profile state")
	}
}

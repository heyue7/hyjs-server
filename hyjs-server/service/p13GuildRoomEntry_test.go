package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestP13GuildRoomEntryOriginalHallAnchorAndSavedReentry(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	now := time.Now()
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("大厅入口")}, 1, now)
	for _, point := range []*pb.PointNew{nil, {X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}} {
		if _, err := f.s.fieldTransfer(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_union"), Point: point}); err != nil {
			t.Fatal(err)
		}
		var row db.Profile
		if err := db.GetIns().First(&row, ids[0]).Error; err != nil {
			t.Fatal(err)
		}
		g := mustP13RoomGame(t, &row)
		if g.Position.Current.GetX() != float32(22.85771) || g.Position.Current.GetY() != float32(1.192649) || g.Position.Current.GetZ() != float32(22.85441) {
			t.Fatal("original TableNPC hall anchor", g.Position.Current)
		}
	}
	explicit := &pb.PointNew{X: proto.Float32(35.101), Y: proto.Float32(-4.689), Z: proto.Float32(-16.417)}
	if _, err := f.s.fieldTransfer(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_union"), Point: explicit}); err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.fieldScene(f.ctx, ids[0], true)
	if err != nil || !proto.Equal(messages[0].(*pb.SCLoginScene).Self.Position.Current, explicit) {
		t.Fatal("valid explicit/relogin point overwritten", err)
	}
	// A malformed original table must fail the position transaction without manufacturing zero coordinates.
	entry := f.s.table("NPC", "npcId", constUtil.P13GuildRoomEntryNPC)
	entry["bornX"] = "bad-original-coordinate"
	if _, err = f.s.fieldTransfer(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_union")}); err == nil {
		t.Fatal("invalid anchor accepted")
	}
	messages, err = f.s.fieldScene(f.ctx, ids[0], true)
	if err != nil || !proto.Equal(messages[0].(*pb.SCLoginScene).Self.Position.Current, explicit) {
		t.Fatal("failed entry changed saved point", err)
	}
	entry["bornX"] = "22.85771"
	entry["sceneName"] = "town"
	if _, err = f.s.P13GuildRoomEntryPoint(nil); err == nil {
		t.Fatal("foreign scene anchor accepted")
	}
}

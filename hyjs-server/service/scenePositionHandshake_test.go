package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestScenePositionHandshake reproduces the actual native old-town move queued
// during the HTTP transfer and subsequent new-room TCP initialization.
func TestScenePositionHandshakeKeepsGuildEntryAcrossOldMovesLoginAndColdStore(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	id := ids[0]
	p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String("原转场移动")}, 1, time.Now())
	var account db.Profile
	if err := db.GetIns().First(&account, id).Error; err != nil {
		t.Fatal(err)
	}
	oldTown := &pb.PointNew{X: proto.Float32(-9.357699), Y: proto.Float32(10.6129045), Z: proto.Float32(37.3301735)}
	request := func(body proto.Message) []proto.Message {
		t.Helper()
		messages, err := f.s.dispatch(f.ctx, &model.GamePacket{PlayerId: id, ZoneId: 1, Session: account.Session, Body: body}, "isolated-native-flow")
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range messages {
			if failure, ok := message.(*pb.ErrorMessage); ok {
				t.Fatal(failure)
			}
		}
		return messages
	}
	read := func() *model.GameState {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		return mustP13RoomGame(t, &row)
	}
	request(&pb.CSTransferScene{Target: proto.String("town_union")})
	entry := proto.Clone(read().Position.Current).(*pb.PointNew)
	if !read().PositionLoading || proto.Equal(entry, oldTown) {
		t.Fatal("transfer did not preserve the original guild entry")
	}
	request(&pb.CSMove{PointNew: oldTown, CurrentTime: proto.Int64(time.Now().UnixMilli())})
	if !proto.Equal(read().Position.Current, entry) {
		t.Fatal("old town movement overwrote new guild entry")
	}
	// Login selects a room before its original OnLoadComplete current-position
	// request; it must retain the pending transition across a cold database reopen.
	messages, err := f.s.fieldScene(f.ctx, id, true)
	if err != nil || !proto.Equal(messages[0].(*pb.SCLoginScene).Self.Position.Current, entry) {
		t.Fatal("login changed the new-room position", err)
	}
	f.open(t)
	request(&pb.CSMove{PointNew: oldTown})
	if !read().PositionLoading || !proto.Equal(read().Position.Current, entry) {
		t.Fatal("cold store lost the pending transition")
	}
	current := request(&pb.CSCurrentPosition{ShowArround: proto.Int32(1)})
	if !proto.Equal(current[0].(*pb.SCCurrentPosition).Self.Position.Current, entry) || read().PositionLoading {
		t.Fatal("loaded-room current-position handshake failed")
	}
	next := proto.Clone(entry).(*pb.PointNew)
	next.X = proto.Float32(entry.GetX() + 1)
	request(&pb.CSMove{PointNew: next})
	if !proto.Equal(read().Position.Current, next) {
		t.Fatal("actual new-room movement remained blocked")
	}
}

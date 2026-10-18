package service

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestApplicationPausePreservesNativeSession 验证后台和恢复通知保持会话及未完成业务，失效会话仍被拒绝。
func TestApplicationPausePreservesNativeSession(t *testing.T) {
	f := p1Setup(t)
	var before db.Profile
	if err := db.GetIns().First(&before, f.id).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range []int32{1, 0, 1, 0} {
		packet := &model.GamePacket{PlayerId: f.id, ZoneId: 1, Session: before.Session, Body: &pb.CSEnterBackgroud{Exit: proto.Int32(state)}}
		reply, err := f.s.dispatch(f.ctx, packet, "127.0.0.1")
		if err != nil || len(reply) != 0 {
			t.Fatalf("one-way pause state %d: reply=%v err=%v", state, reply, err)
		}
	}
	var after db.Profile
	if err := db.GetIns().First(&after, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if before.Player == nil || after.Player == nil || before.Game == nil || after.Game == nil || *before.Player != *after.Player || *before.Game != *after.Game || !bytes.Equal(before.Session, after.Session) {
		t.Fatal("application pause changed profile or session")
	}
	reply, err := f.s.dispatch(f.ctx, &model.GamePacket{PlayerId: f.id, ZoneId: 1, Session: []byte("stale"), Body: &pb.CSEnterBackgroud{}}, "127.0.0.1")
	if err != nil || len(reply) != 1 {
		t.Fatalf("stale pause session: reply=%v err=%v", reply, err)
	}
	if _, ok := reply[0].(*pb.ErrorMessage); !ok {
		t.Fatal("stale session accepted")
	}
}

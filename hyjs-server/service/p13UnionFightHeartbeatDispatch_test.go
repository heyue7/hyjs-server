package service

import (
	"bytes"
	"reflect"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Real paired field entries exercise the original authenticated dispatcher. No
// frame, membership, settlement or battle evidence is manufactured for a ping.
func TestP13UFHeartbeatDispatchNoWritesAndSameSessionReconnect(t *testing.T) {
	f, id, enemy, _, match := p13UFPlayerBattle172ActualSetup(t)
	oldRecorder, peerRecorder := &p10SceneRecorder{}, &p10SceneRecorder{}
	var self db.Profile
	if err := db.GetIns().First(&self, id).Error; err != nil {
		t.Fatal(err)
	}
	oldSession := append([]byte(nil), self.Session...)
	oldCancel := f.s.RegisterP10Client(f.ctx, id, oldSession, oldRecorder.send)
	t.Cleanup(oldCancel)
	var opponent db.Profile
	if err := db.GetIns().First(&opponent, enemy).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.s.RegisterP10Client(f.ctx, enemy, opponent.Session, peerRecorder.send))
	for _, handshake := range []struct {
		opcode int32
		body   proto.Message
	}{
		{30024, &pb.CSCurrentPosition{}}, {30053, &pb.CSLoginScene{}}, {80044, &pb.CSUFInit{}},
	} {
		out := f.s.Dispatch(f.ctx, scene49Packet(t, f, id, handshake.opcode, handshake.body), "127.0.0.1")
		transfer := false
		for _, message := range out {
			if failure, ok := message.(*pb.ErrorMessage); ok {
				t.Fatal("real field handshake", failure.GetMsg())
			}
			if _, ok := message.(*pb.SCUnionFightTransfer); ok {
				transfer = true
			}
		}
		if !transfer {
			t.Fatal("native handshake omitted frozen transfer", handshake.body, out)
		}
	}
	oldRecorder.drain()
	peerRecorder.drain()
	var actorBefore db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actorBefore, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	var profileBefore db.Profile
	if err := db.GetIns().First(&profileBefore, id).Error; err != nil {
		t.Fatal(err)
	}
	factsBefore := p1Count(t, &db.EventFact{}, "player_id IN ?", []int64{id, enemy})
	var profileWrites, actorWrites atomic.Int64
	callback := "p13-heartbeat-no-redundant-write"
	if err := db.GetIns().Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			profileWrites.Add(1)
		}
		if tx.Statement.Table == "p13_union_fight_field_players" {
			actorWrites.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Update().Remove(callback) })
	for i := int64(1); i <= 3; i++ {
		clientTime := int64(12345000) + i
		start := LocalPlayNow(f.ctx).UnixMilli()
		out := f.s.Dispatch(f.ctx, scene49Packet(t, f, id, 30020, &pb.CSPingTcp{ClientTime: proto.Int64(clientTime)}), "127.0.0.1")
		end := LocalPlayNow(f.ctx).UnixMilli()
		pingCount := 0
		for _, message := range out {
			switch value := message.(type) {
			case *pb.ErrorMessage:
				t.Fatal("authenticated war ping rejected", value.GetMsg())
			case *pb.SCPingTcp:
				pingCount++
				if value.GetClientTime() != clientTime || value.GetServerTime() < start || value.GetServerTime() > end {
					t.Fatal("native ping clock/echo changed", value, start, end)
				}
			case *pb.SCUFPlayer, *pb.SCUFPlayerAppear:
				t.Fatal("ping returned duplicate presence", message)
			}
		}
		if pingCount != 1 {
			t.Fatal("native ping response cardinality", out)
		}
		for _, recorder := range []*p10SceneRecorder{oldRecorder, peerRecorder} {
			for _, message := range recorder.drain() {
				switch message.(type) {
				case *pb.SCUFPlayer, *pb.SCUFPlayerAppear:
					t.Fatal("unchanged ping reannounced peers", message)
				}
			}
		}
	}
	if factsAfter := p1Count(t, &db.EventFact{}, "player_id IN ?", []int64{id, enemy}); factsAfter != factsBefore {
		t.Fatal("transport ping created P1 business facts", factsBefore, factsAfter)
	}
	if profileWrites.Load() != 0 || actorWrites.Load() != 0 {
		t.Fatal("unchanged ping wrote profile/actor", profileWrites.Load(), actorWrites.Load())
	}
	if err := db.GetIns().Callback().Update().Remove(callback); err != nil {
		t.Fatal(err)
	}
	var actorAfter db.P13UnionFightFieldPlayer
	var profileAfter db.Profile
	if err := db.GetIns().First(&actorAfter, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&profileAfter, id).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actorBefore, actorAfter) || !reflect.DeepEqual(profileBefore.Game, profileAfter.Game) || !reflect.DeepEqual(profileBefore.Player, profileAfter.Player) {
		t.Fatal("ping changed frozen frame or saved profile")
	}

	// A same-session replacement is a new transport and must receive a real peer
	// snapshot even though its actor's persistent SessionHash is already equal.
	replacement := &p10SceneRecorder{}
	newCancel := f.s.RegisterP10Client(f.ctx, id, oldSession, replacement.send)
	t.Cleanup(newCancel)
	assertPeer := func(messages []proto.Message) {
		t.Helper()
		found := false
		for _, message := range messages {
			if peers, ok := message.(*pb.SCUFPlayer); ok {
				if len(peers.Player) != len(peers.Local) {
					t.Fatal("peer position cardinality")
				}
				for _, player := range peers.Player {
					if player.GetGuid() == enemy {
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatal("same-session reconnect lost actual opponent snapshot", messages)
		}
	}
	assertPeer(replacement.drain())
	oldRecorder.drain()
	peerRecorder.drain()
	oldCancel()
	if _, err := f.s.SyncP13UnionFightField(f.ctx, id, false); err != nil {
		t.Fatal(err)
	}
	assertPeer(replacement.drain())
	if len(oldRecorder.drain()) != 0 {
		t.Fatal("retired sink received replacement snapshot")
	}

	oldPacket := scene49Packet(t, f, id, 30020, &pb.CSPingTcp{ClientTime: proto.Int64(56789)})
	rotated, err := LoginAccount(f.ctx, self.Uid, "isolated-local-pass")
	if err != nil || bytes.Equal(rotated.Session, oldSession) {
		t.Fatal("actual session rotation", err)
	}
	rejected := f.s.Dispatch(f.ctx, oldPacket, "127.0.0.1")
	denied := false
	for _, message := range rejected {
		if _, ok := message.(*pb.ErrorMessage); ok {
			denied = true
		}
		if _, ok := message.(*pb.SCPingTcp); ok {
			t.Fatal("rotated old socket entered ping fast path")
		}
	}
	if !denied {
		t.Fatal("old authenticated packet survived session rotation", rejected)
	}
}

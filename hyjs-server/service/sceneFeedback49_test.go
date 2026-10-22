package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func scene49Packet(t *testing.T, f *p1Fixture, id int64, opcode int32, body proto.Message) *model.GamePacket {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return &model.GamePacket{PlayerId: id, ZoneId: 1, Session: row.Session, Opcode: opcode, Body: body}
}

func TestScene49MoveConfirmIsAuthenticatedOneWay(t *testing.T) {
	f := p1Setup(t)
	packet := scene49Packet(t, f, f.id, 30038, &pb.CSMoveConfirm{})
	var before db.Profile
	db.GetIns().First(&before, f.id)
	for range 3 {
		if out := f.s.Dispatch(f.ctx, packet, "127.0.0.1"); len(out) != 0 {
			t.Fatalf("original movement ack must not trigger a response: %v", out)
		}
	}
	var after db.Profile
	db.GetIns().First(&after, f.id)
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("movement acknowledgement changed saved state")
	}
	packet.Session = []byte("expired-session")
	if out := f.s.Dispatch(f.ctx, packet, "127.0.0.1"); len(out) != 1 {
		t.Fatal("stale movement acknowledgement accepted")
	} else if _, ok := out[0].(*pb.ErrorMessage); !ok {
		t.Fatal("stale movement acknowledgement accepted")
	}
}

func TestScene49CurrentReplaysStationaryPeersAfterFieldLoad(t *testing.T) {
	for _, scene := range []string{"field_001", "field_002"} {
		t.Run(scene, func(t *testing.T) {
			f, other, foreign := p10SceneSetup(t)
			for _, id := range []int64{f.id, other} {
				if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(25); return nil }); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.fieldScene(f.ctx, id, false); err != nil {
					t.Fatal(err)
				}
			}
			ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
			p10SceneConnect(t, f, f.id, ra)
			p10SceneConnect(t, f, other, rb)
			p10SceneConnect(t, f, foreign, rc)
			// The original client clears actors on room load; earlier socket pushes are lost.
			ra.drain()
			rb.drain()
			rc.drain()
			packet := scene49Packet(t, f, f.id, 30024, &pb.CSCurrentPosition{ShowArround: proto.Int32(1)})
			for range 2 {
				out := f.s.Dispatch(f.ctx, packet, "127.0.0.1")
				if len(out) == 0 {
					t.Fatal("missing current scene frame")
				}
				if _, ok := out[0].(*pb.SCCurrentPosition); !ok || !p10SceneHasPlayer(out[1:], other) || p10SceneHasPlayer(out, foreign) {
					t.Fatal("current scene must precede actual same-line peer snapshot")
				}
				for _, message := range out {
					if appear, ok := message.(*pb.SCPlayerAppear); ok {
						for _, actor := range appear.Player {
							if actor.Position.GetScene() != "town_side_"+scene {
								t.Fatal("original client would discard field peer scene", actor.Position.GetScene())
							}
						}
					}
				}
			}
			if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.PositionLoading = true; return nil }); err != nil {
				t.Fatal(err)
			}
			if err := f.s.SyncP10Scene(f.ctx, f.id, false); err != nil {
				t.Fatal(err)
			}
			if !p10SceneHasDisappear(rb.drain(), f.id) {
				t.Fatal("loading field actor remained visible")
			}
		})
	}
}

package service

import (
	"testing"

	pb "hyjs-server/model/protocol"
)

func TestP10SceneUsesTrustedRealmForActualAppearanceAndLookup(t *testing.T) {
	f, target, _ := p10SceneSetup(t)
	f.s.ServerID = 2
	one, two := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, one)
	p10SceneConnect(t, f, target, two)
	appeared := false
	for _, frame := range two.drain() {
		if reply, ok := frame.(*pb.SCPlayerAppear); ok {
			for _, peer := range reply.GetPlayer() {
				if peer.GetInfo().GetPlayerId() == f.id {
					if peer.GetInfo().GetServerId() != 2 {
						t.Fatal("real realm2 peer appeared as another server")
					}
					appeared = true
				}
			}
		}
	}
	if !appeared {
		t.Fatal("actual connected peer was not published")
	}
	replies, handled, err := f.s.DispatchP10Scene(f.ctx, target, &pb.CSMovePlayerInfo{PlayerIds: []int64{f.id}}, 0)
	if err != nil || !handled || len(replies) != 1 {
		t.Fatal("original same-screen lookup failed", err)
	}
	players := replies[0].(*pb.SCMovePlayerInfo).GetPlayerInfos()
	if len(players) != 1 || players[0].GetInfo().GetServerId() != 2 {
		t.Fatal("same-screen lookup lost trusted server identity")
	}
}

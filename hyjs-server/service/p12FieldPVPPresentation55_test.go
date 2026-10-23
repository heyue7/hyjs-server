package service

import (
	"bytes"
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestP12FieldPVPPresentation55BusyTerminalColdAndObservers(t *testing.T) {
	f, peer, observer, sessions := p12FieldPVPSetup(t)
	p12FieldPVPEnable(t)
	f.s.P12FieldPVPReady = true
	if err := updateProfile(f.ctx, peer, func(_ *pb.Player, g *model.GameState) error {
		g.Field.Side.CurHp = proto.Float64(.01)
		g.Field.Side.Anger = proto.Float64(27)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ownRec, peerRec, watchRec := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ownRec)
	disconnect := p10SceneConnect(t, f, peer, peerRec)
	p10SceneConnect(t, f, observer, watchRec)
	ownRec.drain()
	peerRec.drain()
	watchRec.drain()
	now := time.Now()
	messages, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], &pb.CSSidePvpFightResult{PlayerId: proto.Int64(peer)}, 550061, now)
	if err != nil {
		t.Fatal(err)
	}
	assertBusy := func(messages []proto.Message) {
		t.Helper()
		seen := map[int64]*pb.PlayerSide{}
		for _, m := range messages {
			if state, ok := m.(*pb.SCFieldPlayerState); ok {
				for _, side := range state.Player {
					seen[side.GetPlayerId()] = side
				}
			}
		}
		if seen[f.id] == nil || seen[peer] == nil || seen[f.id].GetState() != 6 || seen[peer].GetState() != 7 || seen[peer].GetCurHp() != .01 || seen[peer].GetAnger() != 27 {
			t.Fatal("initial wire skipped active PK", len(seen))
		}
	}
	assertBusy(messages)
	assertBusy(peerRec.drain())
	assertBusy(watchRec.drain())
	snapshot := p12FieldPVPProfiles(t, f.id, peer)
	// Current same-screen reads during the busy lease must not leak the committed death.
	var peerProfile db.Profile
	if err = db.GetIns().First(&peerProfile, peer).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err := decodeProfile(&peerProfile)
	if err != nil {
		t.Fatal(err)
	}
	move, err := f.s.p10SceneMovePlayer(p, g, db.GetIns())
	if err != nil || move.GetInfo().GetPlayerSide().GetState() != 7 || move.GetInfo().GetPlayerSide().GetCurHp() != .01 {
		t.Fatal("same-screen terminal leak", err, move.GetInfo().GetPlayerSide().GetState(), move.GetInfo().GetPlayerSide().GetCurHp())
	}
	if err = f.s.TickP12FieldPVPPresentation(f.ctx, now.Add(5999*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(peerRec.drain()) != 0 || len(watchRec.drain()) != 0 {
		t.Fatal("terminal sent before lease")
	}
	disconnect()
	f.restart(t)
	f.s.P12FieldPVPReady = true
	// A cold process recovers the markers and sends authoritative terminal to currently connected viewers.
	ownRec, watchRec = &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ownRec)
	p10SceneConnect(t, f, observer, watchRec)
	ownRec.drain()
	watchRec.drain()
	if err = f.s.TickP12FieldPVPPresentation(f.ctx, now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	assertTerminal := func(messages []proto.Message) {
		t.Helper()
		seen := map[int64]*pb.PlayerSide{}
		for _, m := range messages {
			if state, ok := m.(*pb.SCFieldPlayerState); ok {
				for _, side := range state.Player {
					seen[side.GetPlayerId()] = side
				}
			}
		}
		for _, id := range []int64{f.id, peer} {
			var row db.Profile
			if e := db.GetIns().First(&row, id).Error; e != nil {
				t.Fatal(e)
			}
			_, g, e := decodeProfile(&row)
			if e != nil {
				t.Fatal(e)
			}
			if seen[id] == nil || !proto.Equal(seen[id], g.Field.Side) {
				t.Fatal("terminal missing/stale", id)
			}
		}
	}
	assertTerminal(ownRec.drain())
	assertTerminal(watchRec.drain())
	if !bytes.Equal(snapshot, p12FieldPVPProfiles(t, f.id, peer)) {
		t.Fatal("display tick changed economic/terminal profile")
	}
	if err = f.s.TickP12FieldPVPPresentation(f.ctx, now.Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(ownRec.drain()) != 0 || len(watchRec.drain()) != 0 {
		t.Fatal("display marker consumed twice")
	}
	var state db.P12FieldPVPState
	if err = db.GetIns().First(&state, "player_id=?", peer).Error; err != nil || state.BusyUntil != 0 {
		t.Fatal("marker not consumed", err)
	}
	var receipt db.P12FieldPVPReceipt
	if err = db.GetIns().First(&receipt, "player_id=?", f.id).Error; err != nil || receipt.BusyUntil != now.UnixMilli()+6000 {
		t.Fatal("immutable receipt lease changed", err)
	}
	// Disconnected victim reads its terminal after reconnect; no replayed economy is needed.
	if err = db.GetIns().First(&peerProfile, peer).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err = decodeProfile(&peerProfile)
	if err != nil {
		t.Fatal(err)
	}
	move, err = f.s.p10SceneMovePlayer(p, g, db.GetIns())
	if err != nil || !proto.Equal(move.GetInfo().GetPlayerSide(), g.Field.Side) {
		t.Fatal("disconnected cold terminal recovery", err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if e := f.s.RunP12FieldPVPPresentation(ctx); e != context.Canceled {
		t.Fatal("worker lifecycle cancellation", e)
	}
}

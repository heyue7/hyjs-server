package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original empty helper replies trigger captured request callbacks. Exercise
// actual assignment replacement, retirement and cold replay with finite actors.
func TestP5HelperReceiptActualReplacementRetirementAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		_, err := f.s.GrantFixedReward(p, g, "hero_011,1;hero_051,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	requests := map[string]*pb.CSHelperOn{}
	for _, w := range g.Warriors {
		if w.GetId() == "hero_011" || w.GetId() == "hero_051" {
			requests[w.GetId()] = &pb.CSHelperOn{Guid: w.Guid, Pos: proto.Int32(1)}
		}
	}
	if len(requests) != 2 {
		t.Fatal("finite original helper actors missing")
	}
	a, b := requests["hero_011"], requests["hero_051"]
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	on := func(request *pb.CSHelperOn, stamp int64) proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP5Next(ctx(request), f.id, request, stamp)
		if !handled || err != nil {
			t.Fatal("actual helper on", handled, err)
		}
		return out[0]
	}
	offRequest := &pb.CSHelperOff{Pos: proto.Int32(1)}
	off := func(stamp int64) proto.Message {
		t.Helper()
		out, err := f.s.p5HelperOff(ctx(offRequest), f.id, offRequest, stamp)
		if err != nil {
			t.Fatal("actual helper off", err)
		}
		return out[0]
	}
	reject := func(request proto.Message, stamp int64) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		var err error
		if request, ok := request.(*pb.CSHelperOn); ok {
			_, _, err = f.s.DispatchP5Next(ctx(request), f.id, request, stamp)
		} else {
			_, err = f.s.p5HelperOff(ctx(request), f.id, offRequest, stamp)
		}
		if err == nil {
			t.Fatal("stale helper acknowledgement changed current client slot")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("rejected helper acknowledgement modified profile")
		}
	}
	first := on(a, 192000)
	f.restart(t)
	if !proto.Equal(first, on(a, 192000)) {
		t.Fatal("current empty on cold changed")
	}
	on(b, 192001)
	reject(a, 192000)
	f.restart(t)
	reject(a, 192000)
	removed := off(192002)
	reject(b, 192001)
	sell := &pb.CSWarriorSell{Guid: []string{b.GetGuid()}}
	if _, err := f.s.p5WarriorRecycle(ctx(sell), f.id, sell.Guid, false, 192003, sell); err != nil {
		t.Fatal("actual retired helper sale", err)
	}
	f.restart(t)
	reject(b, 192001)
	if !proto.Equal(removed, off(192002)) {
		t.Fatal("current empty off cold changed")
	}
	final := on(a, 192004)
	reject(offRequest, 192002)
	f.restart(t)
	reject(offRequest, 192002)
	if !proto.Equal(final, on(a, 192004)) {
		t.Fatal("current final helper cold changed")
	}
	_, g = f.profile(t)
	helpers := P5HelperList(g)
	if len(helpers) != 1 || helpers[0].GetGuid() != a.GetGuid() {
		t.Fatal("actual current helper changed", helpers)
	}
	if _, err := p5Warrior(g, b.GetGuid()); err == nil {
		t.Fatal("retired helper recreated")
	}
}

func TestP5HelperReceiptCurrentGrowthAndEmptyResponseSemantics(t *testing.T) {
	w := &pb.Warrior{Guid: proto.String("held-original-guid"), Id: proto.String("hero_011"), Level: proto.Int32(1)}
	g := &model.GameState{Warriors: []*pb.Warrior{w}}
	p5SetKV(g, "p5.helper:1", w.GetGuid())
	on := &pb.CSHelperOn{Guid: w.Guid, Pos: proto.Int32(1)}
	if err := validateP5HelperReceipt(g, on, &pb.SCHelperOn{}); err != nil {
		t.Fatal(err)
	}
	// No full warrior body is transported: unchanged assignment can retain new growth.
	w.Level = proto.Int32(2)
	if err := validateP5HelperReceipt(g, on, &pb.SCHelperOn{}); err != nil {
		t.Fatal("growth incorrectly expired empty acknowledgement", err)
	}
	g.Warriors = nil
	if err := validateP5HelperReceipt(g, on, &pb.SCHelperOn{}); err == nil {
		t.Fatal("missing held helper accepted")
	}
	if err := validateP5HelperReceipt(g, &pb.CSHelperOff{Pos: proto.Int32(1)}, &pb.SCHelperOff{}); err == nil {
		t.Fatal("old off removed current helper")
	}
	p5SetKV(g, "p5.helper:1", "")
	if err := validateP5HelperReceipt(g, &pb.CSHelperOff{Pos: proto.Int32(1)}, &pb.SCHelperOff{}); err != nil {
		t.Fatal("current off rejected", err)
	}
	if err := validateP5HelperReceipt(g, on, &pb.SCHelperOff{}); err == nil {
		t.Fatal("invalid request/response pair accepted")
	}
	if err := validateP5HelperReceipt(g, &pb.CSItemUse{}, &pb.SCItemUse{}); err != nil {
		t.Fatal("unrelated simple receipt expired", err)
	}
}

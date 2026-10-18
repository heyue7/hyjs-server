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

func TestP5EquipmentFormationReceiptActualOldFrameSwapOffSaleAndCold(t *testing.T) {
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
		_, err := f.s.GrantFixedReward(p, g, "equip_037,2")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	if len(g.Equipment) != 2 {
		t.Fatal("finite gear holdings missing")
	}
	a, b := g.Equipment[0], g.Equipment[1]
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	onA := &pb.CSFormationOn{Id: a.EquipId, Guid: a.Guid, FormatPos: proto.Int32(1)}
	onB := &pb.CSFormationOn{Id: b.EquipId, Guid: b.Guid, FormatPos: proto.Int32(1)}
	offA := &pb.CSFormationOff{Id: a.EquipId, Guid: a.Guid, FormatPos: proto.Int32(1)}
	offB := &pb.CSFormationOff{Id: b.EquipId, Guid: b.Guid, FormatPos: proto.Int32(1)}
	// Mirror the original dispatcher fallback to prove the prior production path.
	call := func(request proto.Message, stamp int64) (proto.Message, error) {
		out, handled, err := f.s.DispatchP5Next(ctx(request), f.id, request, stamp)
		if !handled {
			if onRequest, ok := request.(*pb.CSFormationOn); ok {
				out, err = f.s.equipmentFormationOn(ctx(request), f.id, onRequest)
			} else {
				out, err = f.s.equipmentFormationOff(ctx(request), f.id, request.(*pb.CSFormationOff))
			}
		}
		if err != nil {
			return nil, err
		}
		return out[0], nil
	}
	ok := func(request proto.Message, stamp int64) proto.Message {
		t.Helper()
		out, err := call(request, stamp)
		if err != nil {
			t.Fatal("actual wear frame", err)
		}
		return out
	}
	reject := func(request proto.Message, stamp int64) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, err := call(request, stamp); err == nil {
			t.Fatal("stale equipment frame re-executed old worn GUID")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("old equipment frame modified profile")
		}
	}
	first := ok(onA, 195000)
	f.restart(t)
	if !proto.Equal(first, ok(onA, 195000)) {
		t.Fatal("current worn receipt changed")
	}
	ok(onB, 195001)
	reject(onA, 195000)
	f.restart(t)
	reject(onA, 195000)
	removed := ok(offB, 195002)
	f.restart(t)
	if !proto.Equal(removed, ok(offB, 195002)) {
		t.Fatal("current off cold failed")
	}
	ok(onA, 195003)
	reject(offB, 195002)
	lastOff := ok(offA, 195004)
	f.restart(t)
	if !proto.Equal(lastOff, ok(offA, 195004)) {
		t.Fatal("latest off cold failed")
	}
	sell := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{{Id: a.EquipId, Guid: a.Guid, Num: proto.Int32(1)}}}
	if _, err := f.s.p5EquipmentSell(ctx(sell), f.id, sell, 195005); err != nil {
		t.Fatal(err)
	}
	reject(offA, 195004)
	reject(onA, 195003)
	f.restart(t)
	reject(offA, 195004)
	_, g = f.profile(t)
	if _, err := f.s.equipmentRecord(g, a.GetGuid()); err == nil {
		t.Fatal("retired original equipment returned")
	}
}

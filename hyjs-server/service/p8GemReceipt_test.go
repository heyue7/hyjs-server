package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP8GemReceiptActualMovementGrowthDecomposeAndCold(t *testing.T) {
	f := p1Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking, f.s.InitEquipmentMutations, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.s.P5Ready = true
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(10000000)
		_, e := f.s.GrantFixedReward(p, g, "equip_037,1;MineItem_019,1;MineItem_020,1")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	_, g := f.profile(t)
	gear := g.Equipment[0]
	ctx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	mount := &pb.CSGemMount{Guid: gear.Guid, GemId: proto.String("MineItem_019"), Pos: proto.Int32(0)}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan proto.Message, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.gemMount(ctx(mount), f.id, mount, 1001)
			errs <- e
			if e == nil {
				replies <- out[0]
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(replies)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var mounted proto.Message
	for out := range replies {
		if mounted == nil {
			mounted = out
		}
		if !proto.Equal(out, mounted) {
			t.Fatal("mount8once body changed")
		}
	}
	_, g = f.profile(t)
	if g.Equipment[0].GetGems()[0] != "MineItem_019" {
		t.Fatal("actual mount")
	}
	reject := func(req proto.Message, stamp int64, invoke func() ([]proto.Message, error)) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, e := invoke(); e == nil {
			t.Fatal("retired gem body replayed", req, stamp)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("stale gem replay changed profile")
		}
	}
	strength := &pb.CSEquipStrengthenOneKey{Guid: gear.Guid}
	if _, e := f.s.p5StrengthenOne(ctx(strength), f.id, strength, 2001); e != nil {
		t.Fatal("actual paid equipment growth", e)
	}
	reject(mount, 1001, func() ([]proto.Message, error) { return f.s.gemMount(ctx(mount), f.id, mount, 1001) })
	change := &pb.CSGemMountOrChange{Guid: gear.Guid, GemId: proto.String("MineItem_020")}
	changed, e := f.s.gemMountOrChange(ctx(change), f.id, change, 1002)
	if e != nil {
		t.Fatal(e)
	}
	if changed[0].(*pb.SCGemMountOrChange).GetEquip().GetGems()[0] != "MineItem_020" {
		t.Fatal("actual swap")
	}
	unmount := &pb.CSGemUnmount{Guid: gear.Guid, Pos: proto.Int32(0)}
	unmounted, e := f.s.gemUnmount(ctx(unmount), f.id, unmount, 1003)
	if e != nil {
		t.Fatal(e)
	}
	reject(change, 1002, func() ([]proto.Message, error) { return f.s.gemMountOrChange(ctx(change), f.id, change, 1002) })
	f.restart(t)
	if out, e := f.s.gemUnmount(ctx(unmount), f.id, unmount, 1003); e != nil || !proto.Equal(out[0], unmounted[0]) {
		t.Fatal("current unmount cold receipt", e)
	}
	if _, e = f.s.gemMount(ctx(mount), f.id, mount, 1004); e != nil {
		t.Fatal(e)
	}
	reject(unmount, 1003, func() ([]proto.Message, error) { return f.s.gemUnmount(ctx(unmount), f.id, unmount, 1003) })
	if _, e = f.s.gemMountOrChange(ctx(change), f.id, change, 1005); e != nil {
		t.Fatal(e)
	}
	recycle := &pb.CSEquipDecompose{Guid: []string{gear.GetGuid()}}
	if _, e = f.s.p5EquipmentDecompose(ctx(recycle), f.id, recycle, 3001); e != nil {
		t.Fatal("actual gear decompose", e)
	}
	reject(change, 1005, func() ([]proto.Message, error) { return f.s.gemMountOrChange(ctx(change), f.id, change, 1005) })
	f.restart(t)
	reject(change, 1005, func() ([]proto.Message, error) { return f.s.gemMountOrChange(ctx(change), f.id, change, 1005) })
	reject(unmount, 1003, func() ([]proto.Message, error) { return f.s.gemUnmount(ctx(unmount), f.id, unmount, 1003) })
	_, g = f.profile(t)
	counts := map[string]int32{}
	for _, item := range g.Gems {
		counts[item.GetId()] += item.GetNum()
	}
	if len(g.Equipment) != 0 || counts["MineItem_019"] != 1 || counts["MineItem_020"] != 1 {
		t.Fatal("real physicalgem conservation afterdecompose", counts)
	}
}

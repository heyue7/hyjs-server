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

func TestP5EmptyGemSaleOriginalUnmountRefundMixedAndCold(t *testing.T) {
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
		_, e := f.s.GrantFixedReward(p, g, "equip_037,2;MineItem_019,2")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	ctx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	p, g := f.profile(t)
	gears := g.Equipment
	mount := func(gear *pb.Equipment, stamp int64) {
		t.Helper()
		r := &pb.CSGemMount{Guid: gear.Guid, GemId: proto.String("MineItem_019"), Pos: proto.Int32(0)}
		if _, e := f.s.gemMount(ctx(r), f.id, r, stamp); e != nil {
			t.Fatal(e)
		}
	}
	for i, gear := range gears {
		mount(gear, 4001+int64(i))
	}
	strength := &pb.CSEquipStrengthenOneKey{Guid: gears[0].Guid}
	if _, e := f.s.p5StrengthenOne(ctx(strength), f.id, strength, 5001); e != nil {
		t.Fatal("actualtrackedgrowth", e)
	}
	p, g = f.profile(t)
	grown, _ := f.s.equipmentRecord(g, gears[0].GetGuid())
	ledger, e := p5InvestmentState(g, grown.GetGuid(), grown)
	if e != nil {
		t.Fatal(e)
	}
	var invested int64
	for _, cost := range ledger.Costs {
		if cost.GetId() == "coin" {
			invested += int64(cost.GetNum())
		}
	}
	unmount := &pb.CSGemUnmount{Guid: gears[0].Guid, Pos: proto.Int32(0)}
	if _, e := f.s.gemUnmount(ctx(unmount), f.id, unmount, 6001); e != nil {
		t.Fatal(e)
	}
	p, g = f.profile(t)
	grown, _ = f.s.equipmentRecord(g, gears[0].GetGuid())
	if len(grown.GetGems()) != 4 {
		t.Fatal("originalfouremptyslotshape", grown.GetGems())
	}
	for _, gem := range grown.GetGems() {
		if gem != "" {
			t.Fatal("notreallyunmounted")
		}
	}
	sell := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{{Id: gears[0].EquipId, Guid: gears[0].Guid, Num: proto.Int32(1)}, {Id: gears[1].EquipId, Guid: gears[1].Guid, Num: proto.Int32(1)}}}
	beforeP, beforeG := f.profile(t)
	before, _ := json.Marshal(beforeG)
	if _, e := f.s.p5EquipmentSell(ctx(sell), f.id, sell, 7001); e == nil {
		t.Fatal("mixedoccupiedbatchallowed")
	}
	afterP, afterG := f.profile(t)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("mixedsale partiallyrefunded orremoved gear")
	}
	sell.Cost = sell.Cost[:1]
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan proto.Message, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.p5EquipmentSell(ctx(sell), f.id, sell, 7002)
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
			t.Fatal("actual unmounted sale", e)
		}
	}
	var sold proto.Message
	for out := range replies {
		if sold == nil {
			sold = out
		}
		if !proto.Equal(out, sold) {
			t.Fatal("eightway sale differ")
		}
	}
	finalP, finalG := f.profile(t)
	if finalP.GetCoin() != p.GetCoin()+invested+1000 || len(finalG.Equipment) != 1 || finalG.Equipment[0].GetGuid() != gears[1].GetGuid() {
		t.Fatal("exactrefund/basecoin/unaffectedattachedgear")
	}
	f.restart(t)
	if out, e := f.s.p5EquipmentSell(ctx(sell), f.id, sell, 7002); e != nil || !proto.Equal(out[0], sold) {
		t.Fatal("simple refund coldsale receipt", e)
	}
	beforeP, beforeG = f.profile(t)
	before, _ = json.Marshal(beforeG)
	if _, e := f.s.gemUnmount(ctx(unmount), f.id, unmount, 6001); e == nil {
		t.Fatal("oldunmount revivedsold equipment")
	}
	afterP, afterG = f.profile(t)
	after, _ = json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("oldunmount changed soldprofile")
	}
}

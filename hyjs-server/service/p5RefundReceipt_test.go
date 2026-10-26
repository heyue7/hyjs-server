package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP5RefundMaterialReceiptHeldConsumedAndCold(t *testing.T) {
	f := p1Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.s.P5Ready = true
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(1000000)
		p.WarriorBagSize = proto.Int32(1000)
		_, e := f.s.GrantFixedReward(p, g, "item_067,4;hero_exp_002,2")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	requestCtx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	for scenario := int64(0); scenario < 2; scenario++ {
		stamp := int64(900000) + scenario*100
		use := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_067"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(1)}
		var guids []string
		for i := 0; i < 2; i++ {
			stamp++
			out, e := f.s.p5BoxUse(requestCtx(use), f.id, use, stamp)
			if e != nil {
				t.Fatal(e)
			}
			guids = append(guids, out[0].(*pb.SCItemUse).GetGet().GetWars()[0].GetGuid())
		}
		_, g := f.profile(t)
		var material *pb.Warrior
		for _, w := range g.Warriors {
			if w.GetId() == "hero_exp_002" {
				material = w
				break
			}
		}
		if material == nil {
			t.Fatal("actual experience qualifier missing")
		}
		grow := &pb.CSWarriorStrengthen{Guid: proto.String(guids[0]), Cost: []*pb.BaseResource{{Id: material.Id, Guid: material.Guid, Num: proto.Int32(1)}}}
		if _, e := f.s.warriorStrengthen(requestCtx(grow), f.id, grow); e != nil {
			t.Fatal(e)
		}
		stamp++
		decompose := scenario == 1
		var recycle proto.Message = &pb.CSWarriorSell{Guid: []string{guids[0]}}
		if decompose {
			recycle = &pb.CSWarriorDecompose{Guid: []string{guids[0]}}
		}
		out, e := f.s.p5WarriorRecycle(requestCtx(recycle), f.id, []string{guids[0]}, decompose, stamp, recycle)
		if e != nil {
			t.Fatal("actual paid recycle/refund", e)
		}
		var refund *pb.ResultObject
		if decompose {
			refund = out[0].(*pb.SCWarriorDecompose).GetGet()
		} else {
			refund = out[0].(*pb.SCWarriorSell).GetGet()
		}
		if len(refund.GetWars()) != 1 || refund.GetWars()[0].GetId() != "hero_exp_002" {
			t.Fatal("actual 5000-EXP normalized material refund", refund)
		}
		if again, e := f.s.p5WarriorRecycle(requestCtx(recycle), f.id, []string{guids[0]}, decompose, stamp, recycle); e != nil || !proto.Equal(again[0], out[0]) {
			t.Fatal("held refund old receipt", e)
		}
		returned := refund.GetWars()[0]
		consume := &pb.CSWarriorStrengthen{Guid: proto.String(guids[1]), Cost: []*pb.BaseResource{{Id: returned.Id, Guid: returned.Guid, Num: proto.Int32(1)}}}
		if _, e := f.s.warriorStrengthen(requestCtx(consume), f.id, consume); e != nil {
			t.Fatal("actual refund material consumed", e)
		}
		beforeP, beforeG := f.profile(t)
		beforeRaw, _ := json.Marshal(beforeG)
		if _, e := f.s.p5WarriorRecycle(requestCtx(recycle), f.id, []string{guids[0]}, decompose, stamp, recycle); e == nil {
			t.Fatal("consumed material resurrected by recycle receipt")
		}
		f.restart(t)
		if _, e := f.s.p5WarriorRecycle(requestCtx(recycle), f.id, []string{guids[0]}, decompose, stamp, recycle); e == nil {
			t.Fatal("consumed refund resurrected cold")
		}
		afterP, afterG := f.profile(t)
		afterRaw, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
			t.Fatal("rejected refund replay changed saved profile")
		}
	}
}

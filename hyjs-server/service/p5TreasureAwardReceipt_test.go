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

// Original fragment and box holdings are finite qualification. The actual
// Combine, random four-result box, consuming strength and sale create/remove
// every tested GUID through the existing business operations.
func TestP5TreasureAwardsCurrentConsumedGrownSoldAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitializeTreasureBoxes} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(260000)
		p.EquipBagSize = proto.Int32(1000)
		_, err := f.s.GrantFixedReward(p, g, "frag_trea_001_1,1;frag_trea_001_2,1;frag_trea_001_3,1;item_115,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	combine := &pb.CSCombine{TargetId: proto.String("trea_001")}
	combined, err := f.s.treasureCombine(ctx(combine), f.id, combine, 185000)
	if err != nil {
		t.Fatal(err)
	}
	target := combined[0].(*pb.SCCombine).GetGet().GetTreas()[0]
	use := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_115"), Num: proto.Int32(1)}}
	opened, err := f.s.treasureBoxUse(ctx(use), f.id, use, 185001)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened[0].(*pb.SCItemUse).GetGet().GetTreas()) != 1 {
		t.Fatal("original four-result115 source missing")
	}
	food := opened[0].(*pb.SCItemUse).GetGet().GetTreas()[0]
	f.restart(t)
	if again, err := f.s.treasureCombine(ctx(combine), f.id, combine, 185000); err != nil || !proto.Equal(combined[0], again[0]) {
		t.Fatal("current combine cold", err)
	}
	if again, err := f.s.treasureBoxUse(ctx(use), f.id, use, 185001); err != nil || !proto.Equal(opened[0], again[0]) {
		t.Fatal("current box cold", err)
	}
	growth := &pb.CSTreasureStrengthen{TargetId: target.Guid, Guids: []string{food.GetGuid()}}
	if _, err := f.s.treasureStrengthen(ctx(growth), f.id, growth); err != nil {
		t.Fatal("actual consuming strength", err)
	}
	deny := func() {
		t.Helper()
		p, g := f.profile(t)
		before, _ := json.Marshal(g)
		if _, err := f.s.treasureCombine(ctx(combine), f.id, combine, 185000); err == nil {
			t.Fatal("old combine overwrote actual strength")
		}
		if _, err := f.s.treasureBoxUse(ctx(use), f.id, use, 185001); err == nil {
			t.Fatal("old box resurrected consumed treasure")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(p, afterP) || string(before) != string(after) {
			t.Fatal("rejected treasure awards changed save")
		}
	}
	deny()
	f.restart(t)
	deny()
	sell := &pb.CSTreasureSell{Guids: []string{target.GetGuid()}}
	if _, err := f.s.p5TreasureSell(ctx(sell), f.id, sell, 185002); err != nil {
		t.Fatal("actual grown sale/refund", err)
	}
	deny()
	f.restart(t)
	deny()
	p, g := f.profile(t)
	if p.GetCoin() != 261000 || len(g.Treasures) != 1 || g.Treasures[0].GetGuid() == target.GetGuid() || g.Treasures[0].GetGuid() == food.GetGuid() {
		t.Fatal("full actual refund/baseprice/newrefund GUID conservation", p.GetCoin(), g.Treasures)
	}
}

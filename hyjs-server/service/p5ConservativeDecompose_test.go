package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP5ConservativeDecomposeAndCopyRefund binds approved values to original Hero qualities.
func TestP5ConservativeDecomposeAndCopyRefund(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id          string
		light, soul int32
	}{
		{"hero_056", 1, 0}, {"hero_032", 5, 0}, {"hero_020", 50, 0}, {"hero_007", 200, 0}, {"hero_001", 500, 1},
	} {
		rewards, err := s.p5WarriorDecomposeReward(tc.id, 2)
		amounts := map[string]int32{}
		for _, r := range rewards {
			amounts[r.GetId()] += r.GetNum()
		}
		if err != nil || amounts["light"] != tc.light*2 || amounts["item_020"] != tc.soul*2 || len(amounts) > 2 {
			t.Fatalf("%s: %v %v", tc.id, amounts, err)
		}
	}
	if _, err := s.p5WarriorDecomposeReward("hero_001", math.MaxInt32); err == nil {
		t.Fatal("overflow copy count accepted")
	}
	state := &p5Investment{RefundVersion: 1, Costs: []*pb.BaseResource{
		{Id: proto.String("hero_001"), Num: proto.Int32(3)}, {Id: proto.String("coin"), Num: proto.Int32(100)}, {Id: proto.String("item_005"), Num: proto.Int32(2)},
	}, NonRefundable: map[string]int64{"coin": 100}, WarriorCopies: map[string]int64{"hero_001": 3}}
	rewards, err := s.p5WarriorRefundCosts(&model.GameState{}, state, &pb.Warrior{Id: proto.String("hero_001")}, true)
	amounts := map[string]int32{}
	for _, r := range rewards {
		amounts[r.GetId()] += r.GetNum()
	}
	if err != nil || amounts["light"] != 1500 || amounts["item_020"] != 3 || amounts["item_005"] != 2 || amounts["coin"] != 0 || amounts["frag_hero_001"] != 0 || amounts["hero_001"] != 0 {
		t.Fatal("same-copy conversion or paid-cost exclusions", amounts, err)
	}
}

// TestP5ConservativeDecomposeTransaction uses finite owned cards and a classified paid-ledger fixture.
// The public mutation validates ownership, full rollback, exact retry and cold persisted settlement.
func TestP5ConservativeDecomposeTransaction(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	var red, blue string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		_, err := f.s.GrantFixedReward(p, g, "hero_001,1;hero_032,1")
		if err != nil {
			return err
		}
		for _, w := range g.Warriors {
			if w.GetId() == "hero_032" {
				blue = w.GetGuid()
			}
			if w.GetId() != "hero_001" {
				continue
			}
			red = w.GetGuid()
			baseline := proto.Clone(w).(*pb.Warrior)
			w.Talent = proto.Int32(1)
			// A bounded classified fixture represents three already consumed same-name talent cards.
			if err := RecordP5Investment(g, red, baseline, w, []*pb.BaseResource{
				{Id: proto.String("hero_001"), Num: proto.Int32(3)}, {Id: proto.String("item_005"), Num: proto.Int32(2)}, {Id: proto.String("coin"), Num: proto.Int32(100)},
			}, "CSWarriorTalent"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if red == "" || blue == "" {
		t.Fatal("finite cards missing")
	}
	request := &pb.CSWarriorDecompose{Guid: []string{red, blue}}
	ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	beforeP, beforeG := f.profile(t)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Light = proto.Int32(math.MaxInt32); return nil }); err != nil {
		t.Fatal(err)
	}
	overflowP, overflowG := f.profile(t)
	overflowRaw, _ := json.Marshal(overflowG)
	if _, err := f.s.p5WarriorRecycle(ctx, f.id, request.GetGuid(), true, 989001, request); err == nil {
		t.Fatal("light capacity overflow accepted")
	}
	stillP, stillG := f.profile(t)
	stillRaw, _ := json.Marshal(stillG)
	if !proto.Equal(overflowP, stillP) || string(overflowRaw) != string(stillRaw) {
		t.Fatal("overflow changed card, ledger or rewards")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Light = beforeP.Light; return nil }); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.p5WarriorRecycle(ctx, f.id, request.GetGuid(), true, 989001, request)
	if err != nil {
		t.Fatal(err)
	}
	afterP, afterG := f.profile(t)
	if afterP.GetLight()-beforeP.GetLight() != 2005 || afterP.GetCoin() != beforeP.GetCoin() || simpleOwned(afterP, afterG, "item_020")-simpleOwned(beforeP, beforeG, "item_020") != 4 || simpleOwned(afterP, afterG, "item_005")-simpleOwned(beforeP, beforeG, "item_005") != 2 || simpleOwned(afterP, afterG, "frag_hero_001") != simpleOwned(beforeP, beforeG, "frag_hero_001") {
		t.Fatal("approved decomposition or ordinary refund mismatch")
	}
	for _, w := range afterG.Warriors {
		if w.GetGuid() == red || w.GetGuid() == blue {
			t.Fatal("decomposed card retained")
		}
	}
	if len(out[0].(*pb.SCWarriorDecompose).GetCost()) != 2 {
		t.Fatal("original client removal contract")
	}
	saved, _ := json.Marshal(afterG)
	f.restart(t)
	again, err := f.s.p5WarriorRecycle(ctx, f.id, request.GetGuid(), true, 989001, request)
	if err != nil || !proto.Equal(again[0], out[0]) {
		t.Fatal("cold receipt", err)
	}
	coldP, coldG := f.profile(t)
	coldRaw, _ := json.Marshal(coldG)
	if !proto.Equal(afterP, coldP) || string(saved) != string(coldRaw) {
		t.Fatal("cold decomposition paid twice")
	}
}

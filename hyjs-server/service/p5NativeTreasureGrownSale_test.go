package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// These finite vectors verify the original cash equation and fail-closed
// ledger rules; they do not claim a natural material producer.
func TestP5NativeTreasureSaleGrownFormulaAndLedgerProtection(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	p := &pb.Player{}
	for _, id := range []string{"trea_001", "trea_006"} {
		for _, level := range []int32{1, 2, 50, 100} {
			before := &pb.Treasure{Guid: proto.String("vector"), TreasureId: proto.String(id), StrengthLevel: proto.Int32(1), RefineLevel: proto.Int32(0), CurrentExp: proto.Int32(0)}
			after := proto.Clone(before).(*pb.Treasure)
			after.StrengthLevel, after.CurrentExp = proto.Int32(level), proto.Int32(7)
			g := &model.GameState{Treasures: []*pb.Treasure{after}}
			if err := RecordP5Investment(g, "vector", before, after, []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(100)}, {Id: proto.String("trea_005"), Num: proto.Int32(101)}}); err != nil {
				t.Fatal(err)
			}
			cost := &pb.BaseResource{Id: after.TreasureId, Guid: after.Guid, Num: proto.Int32(1)}
			var want int64 = 1007
			for _, row := range s.Tables["TreasureStrengthLevelExp"] {
				if row["quality"] == s.table("Treasure", "id", id)["quality"] && tableNumber(row, "level") >= 1 && tableNumber(row, "level") < float64(level) {
					want += int64(tableNumber(row, "exp"))
				}
			}
			got, err := s.p5NativeTreasureSalePrice(p, g, cost)
			if err != nil || got != want {
				t.Fatal(id, level, "original accumulated EXP cash", got, want, err)
			}
			for _, mode := range []string{"missing", "changed-state", "partial-origin", "wrong-origin", "bad-cost", "settled", "overflow"} {
				t.Run(fmt.Sprintf("%s-%d-%s", id, level, mode), func(t *testing.T) {
					copy := &model.GameState{Treasures: []*pb.Treasure{proto.Clone(after).(*pb.Treasure)}}
					p5SetKV(copy, "p5.invest:vector", p5KV(g, "p5.invest:vector"))
					state, err := p5InvestmentState(copy, "vector", copy.Treasures[0])
					if err != nil {
						t.Fatal(err)
					}
					switch mode {
					case "missing":
						p5SetKV(copy, "p5.invest:vector", "")
					case "changed-state":
						copy.Treasures[0].CurrentExp = proto.Int32(8)
					case "partial-origin", "wrong-origin":
						base := proto.Clone(before).(*pb.Treasure)
						if mode == "partial-origin" {
							base.StrengthLevel = proto.Int32(2)
						} else {
							base.Guid = proto.String("other-owner-instance")
						}
						state.Baseline, _ = json.Marshal(base)
					case "bad-cost":
						state.Costs[0].Num = proto.Int32(-1)
					case "settled":
						p5SetKV(copy, "p5.native-sale:vector", "already liquidated")
					case "overflow":
						copy.Treasures[0].CurrentExp = proto.Int32(math.MaxInt32)
						state.Expected, _ = p5InvestmentFingerprint(copy.Treasures[0])
					}
					if mode != "missing" {
						raw, _ := json.Marshal(state)
						p5SetKV(copy, "p5.invest:vector", string(raw))
					}
					raw, _ := json.Marshal(copy)
					if _, err := s.p5NativeTreasureSalePrice(p, copy, cost); err == nil {
						t.Fatal("incomplete or stale investment accepted")
					}
					now, _ := json.Marshal(copy)
					if string(now) != string(raw) {
						t.Fatal("quote modified state")
					}
				})
			}
		}
	}
}

// Finite original materials exercise real strengthen/refine consumers. Source
// acquisition is independently covered by original022 HTTP; this test does not
// relabel the fixture's resources as a natural new-account acquisition chain.
func TestP5NativeTreasureSaleGrownMixedLiquidationAtomicRetryAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	var target, donor *pb.Treasure
	var equipment *pb.Equipment
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		// Finite reset qualification makes the post-sale reset rejection test
		// reach deleted-instance protection rather than the unrelated level gate.
		p.Level = proto.Int32(int32(tableNumber(f.s.table("GameConfigure", "StaticName", "RecycleRoomLevel"), "NameValue")))
		gain, err := f.s.GrantFixedReward(p, g, "coin,100000;diamond,50;item_012,30;trea_001,1;trea_005,1;equip_005,1;trea_011,1")
		if err == nil {
			for _, v := range gain.GetTreas() {
				if v.GetTreasureId() == "trea_001" {
					target = v
				}
				if v.GetTreasureId() == "trea_005" {
					donor = v
				}
			}
			equipment = gain.GetEquips()[0]
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	strength := &pb.CSTreasureStrengthen{TargetId: target.Guid, Guids: []string{donor.GetGuid()}}
	if _, err := f.s.p5TreasureStrengthen(ctx(strength), f.id, strength, 94001); err != nil {
		t.Fatal(err)
	}
	refine := &pb.CSTreasureRefine{TargetId: target.Guid}
	if _, err := f.s.treasureRefine(ctx(refine), f.id, refine, 94002); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	current, err := f.s.treasureRecord(g, target.GetGuid())
	if err != nil || current.GetRefineLevel() != 1 || current.GetStrengthLevel() < 2 {
		t.Fatal("real grown/refined state", current, err)
	}
	ledger := p5KV(g, "p5.invest:"+target.GetGuid())
	if _, err := p5InvestmentState(g, target.GetGuid(), current); err != nil {
		t.Fatal("real consumer ledger", err)
	}
	cost := &pb.BaseResource{Id: target.TreasureId, Guid: target.Guid, Num: proto.Int32(1)}
	price, err := f.s.p5NativeTreasureSalePrice(p, g, cost)
	if err != nil || price != 3500 {
		t.Fatal("original donor2500 EXP plus base1000, no refinement refund", price, err)
	}
	coinBefore := p.GetCoin()
	before, _ := json.Marshal([]any{p, g})
	bad := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{cost, {Id: proto.String("trea_011"), Guid: proto.String("wrong-guid"), Num: proto.Int32(1)}}}
	if _, err := f.s.p5EquipmentSell(ctx(bad), f.id, bad, 94003); err == nil {
		t.Fatal("invalid mixed batch accepted")
	}
	p, g = f.profile(t)
	now, _ := json.Marshal([]any{p, g})
	if string(now) != string(before) {
		t.Fatal("mixed rejection changed investment/audit/coin")
	}
	sale := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{cost, {Id: equipment.EquipId, Guid: equipment.Guid, Num: proto.Int32(1)}}}
	var wg sync.WaitGroup
	results := make(chan *pb.SCEquipmentSell, 8)
	errors := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.p5EquipmentSell(ctx(sale), f.id, sale, 94004)
			if err != nil {
				errors <- err
				return
			}
			results <- out[0].(*pb.SCEquipmentSell)
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var receipt *pb.SCEquipmentSell
	count := 0
	for r := range results {
		count++
		if receipt == nil {
			receipt = r
		} else if !proto.Equal(receipt, r) {
			t.Fatal("same stamp changed response")
		}
	}
	if count != 8 || len(receipt.GetGet().GetSimps()) != 1 || receipt.GetGet().GetSimps()[0].GetId() != "coin" || receipt.GetGet().GetSimps()[0].GetNum() != 4500 || len(receipt.GetGet().GetTreas()) != 0 {
		t.Fatal("cash-only original response", receipt)
	}
	assert := func() {
		t.Helper()
		p, g := f.profile(t)
		if p.GetCoin() != coinBefore+4500 || p.GetDiamond() != 50 || simpleOwned(p, g, "item_012") != 0 || p5KV(g, "p5.invest:"+target.GetGuid()) != "" {
			t.Fatal("paid costs refunded or sale paid twice")
		}
		for _, v := range g.Treasures {
			if v.GetGuid() == target.GetGuid() || v.GetGuid() == donor.GetGuid() {
				t.Fatal("sold/consumed treasure resurrected")
			}
		}
		var audit p5NativeTreasureSaleSettlement
		if json.Unmarshal([]byte(p5KV(g, "p5.native-sale:"+target.GetGuid())), &audit) != nil || audit.Policy != constUtil.P5NativeTreasureSalePolicy || audit.Cash != 3500 || string(audit.Investment) != ledger {
			t.Fatal("exact investment audit missing", audit)
		}
	}
	assert()
	f.restart(t)
	assert()
	out, err := f.s.p5EquipmentSell(ctx(sale), f.id, sale, 94004)
	if err != nil || !proto.Equal(out[0], receipt) {
		t.Fatal("cold sale receipt", err)
	}
	if _, err := f.s.p5TreasureStrengthen(ctx(strength), f.id, strength, 94001); err == nil {
		t.Fatal("old strengthen receipt resurrects sold GUID")
	}
	if _, err := f.s.treasureRefine(ctx(refine), f.id, refine, 94002); err == nil {
		t.Fatal("old refine receipt resurrects sold GUID")
	}
	reset := &pb.CStreasureRebuild{TargetId: target.Guid}
	if _, err := f.s.p5TreasureReset(ctx(reset), f.id, reset, 94005); err == nil {
		t.Fatal("settled investment refunded by reset")
	}
	if _, err := f.s.p5EquipmentSell(ctx(sale), f.id, sale, 94006); err == nil {
		t.Fatal("new stamp resold deleted GUID")
	}
	assert()
}

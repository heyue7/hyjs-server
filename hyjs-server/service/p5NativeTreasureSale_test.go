package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP5NativeTreasureSaleOriginalEligibilityAndInvestmentProtection(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	player := &pb.Player{HeroWarId: proto.String("main")}
	makeState := func(id string) (*model.GameState, *pb.BaseResource) {
		treasure := &pb.Treasure{TreasureId: proto.String(id), Guid: proto.String("owned-low"), StrengthLevel: proto.Int32(1), RefineLevel: proto.Int32(0), CurrentExp: proto.Int32(0)}
		return &model.GameState{Treasures: []*pb.Treasure{treasure}}, &pb.BaseResource{Id: proto.String(id), Guid: treasure.Guid, Num: proto.Int32(1)}
	}
	admitted := 0
	for _, row := range s.Tables["Treasure"] {
		g, cost := makeState(row["id"])
		price, err := s.p5NativeTreasureSalePrice(player, g, cost)
		if tableNumber(row, "quality") <= 18 {
			admitted++
			if err != nil || price != 1000 {
				t.Fatal("original ten low-tier sale prices", row["id"], price, err)
			}
		} else if err == nil {
			t.Fatal("original orange/red UI protection bypassed", row["id"])
		}
	}
	if admitted != 10 {
		t.Fatal("original low-tier denominator changed", admitted)
	}
	for _, mode := range []string{"wrong-id", "wrong-num", "missing-guid", "duplicate-guid", "cross-type-guid", "mounted", "locked", "dispatched", "strengthened", "refined", "exp-remainder", "unreturned-investment"} {
		t.Run(mode, func(t *testing.T) {
			g, cost := makeState("trea_001")
			switch mode {
			case "wrong-id":
				cost.Id = proto.String("trea_003")
			case "wrong-num":
				cost.Num = proto.Int32(2)
			case "missing-guid":
				cost.Guid = proto.String("not-owned")
			case "duplicate-guid":
				g.Treasures = append(g.Treasures, proto.Clone(g.Treasures[0]).(*pb.Treasure))
			case "cross-type-guid":
				g.Equipment = []*pb.Equipment{{EquipId: proto.String("equip_005"), Guid: cost.Guid, Level: proto.Int32(1)}}
			case "mounted":
				g.Formation = []*pb.FormationItem{{Things: []*pb.FormationThing{{Id: cost.Id, Guid: cost.Guid}}}}
			case "locked":
				p5SetKV(g, "p5.lock:"+cost.GetGuid(), "1")
			case "dispatched":
				p5SetKV(g, "p5.dispatch:"+cost.GetGuid(), "active")
			case "strengthened":
				g.Treasures[0].StrengthLevel = proto.Int32(2)
			case "refined":
				g.Treasures[0].RefineLevel = proto.Int32(1)
			case "exp-remainder":
				g.Treasures[0].CurrentExp = proto.Int32(1)
			case "unreturned-investment":
				p5SetKV(g, "p5.invest:"+cost.GetGuid(), "paid investment stays protected")
			}
			before, _ := json.Marshal(g)
			if _, err := s.p5NativeTreasureSalePrice(player, g, cost); err == nil {
				t.Fatal("protected or forged original sale accepted")
			}
			after, _ := json.Marshal(g)
			if string(before) != string(after) {
				t.Fatal("quote changed the protected instance or ledger")
			}
		})
	}
}

// Finite diamonds and one baseline blue equipment are domain prerequisites.
// All tested treasure GUIDs are produced by the unchanged paid original022
// box, with its full random pool. No reward row, random draw or price is edited.
func TestP5NativeTreasureSaleOriginal022MixedAtomicAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitializeShop, f.s.InitializeTreasureBoxes} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	var equipment *pb.Equipment
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Diamond = proto.Int64(1000)
		gain, err := f.s.GrantFixedReward(p, g, "equip_005,1")
		if err == nil {
			equipment = gain.GetEquips()[0]
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	requestContext := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_14"), BuyCount: proto.Int32(20)}
	if _, err := f.s.shopBuy(requestContext(buy), f.id, buy, 93001); err != nil {
		t.Fatal("original022 twenty paid5-diamond boxes", err)
	}
	bought, _ := f.profile(t)
	if bought.GetDiamond() != 900 {
		t.Fatal("original022 cost changed", bought.GetDiamond())
	}
	use := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_022"), Num: proto.Int32(20)}}
	opened, err := f.s.treasureBoxUse(requestContext(use), f.id, use, 93002)
	if err != nil {
		t.Fatal("unchanged full original022 pool", err)
	}
	box := opened[0].(*pb.SCItemUse)
	var target *pb.Treasure
	for _, treasure := range box.GetGet().GetTreas() {
		if tableNumber(f.s.table("Treasure", "id", treasure.GetTreasureId()), "quality") <= 18 {
			target = treasure
			break
		}
	}
	if target == nil {
		t.Fatal("bounded twenty genuine022 draws produced no low-tier sample; preserve outcome and do not force a reward")
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_022") != 0 {
		t.Fatal("original box was not consumed")
	}
	treasureCost := &pb.BaseResource{Id: target.TreasureId, Guid: target.Guid, Num: proto.Int32(1)}
	equipmentCost := &pb.BaseResource{Id: equipment.EquipId, Guid: equipment.Guid, Num: proto.Int32(1)}
	reject := func(request *pb.CSEquipmentSell, stamp int64) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal([]any{beforeP, beforeG})
		if _, err := f.s.p5EquipmentSell(requestContext(request), f.id, request, stamp); err == nil {
			t.Fatal("invalid mixed original sale accepted")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal([]any{afterP, afterG})
		if string(before) != string(after) {
			t.Fatal("failed mixed sale changed currency or any instance")
		}
	}
	reject(&pb.CSEquipmentSell{Cost: []*pb.BaseResource{equipmentCost, treasureCost, treasureCost}}, 93003)
	forged := proto.Clone(treasureCost).(*pb.BaseResource)
	forged.Id = proto.String("trea_999")
	reject(&pb.CSEquipmentSell{Cost: []*pb.BaseResource{equipmentCost, forged}}, 93004)
	forged = proto.Clone(treasureCost).(*pb.BaseResource)
	forged.Num = proto.Int32(2)
	reject(&pb.CSEquipmentSell{Cost: []*pb.BaseResource{equipmentCost, forged}}, 93005)
	request := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{equipmentCost, treasureCost}}
	var workers sync.WaitGroup
	replies := make(chan proto.Message, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			out, e := f.s.p5EquipmentSell(requestContext(request), f.id, request, 93006)
			if e != nil {
				failures <- e
				return
			}
			if len(out) == 0 {
				failures <- fmt.Errorf("empty original sale response")
				return
			}
			replies <- out[0]
		}()
	}
	workers.Wait()
	close(replies)
	close(failures)
	for failure := range failures {
		t.Fatal("eight original mixed-sale retries", failure)
	}
	var sold *pb.SCEquipmentSell
	count := 0
	for response := range replies {
		count++
		if sold == nil {
			sold = response.(*pb.SCEquipmentSell)
		} else if !proto.Equal(sold, response) {
			t.Fatal("same-stamp sale callback changed")
		}
	}
	if count != 8 || len(sold.GetCost()) != 2 || !proto.Equal(sold.GetCost()[1], treasureCost) || len(sold.GetGet().GetSimps()) != 1 || len(sold.GetGet().GetTreas()) != 0 || len(sold.GetGet().GetEquips()) != 0 {
		t.Fatal("original SCEquipmentSell cost/delete + coin-only AddResultObj contract", sold)
	}
	expectedCoin := int64(tableNumber(f.s.table("Equip", "id", equipment.GetEquipId()), "sellPrice")) + 1000
	final, finalGame := f.profile(t)
	if final.GetCoin()-p.GetCoin() != expectedCoin || final.GetDiamond() != 900 {
		t.Fatal("mixed sale paid more than original base prices")
	}
	if final.GetEquipBagSize() != p.GetEquipBagSize() || len(finalGame.Treasures) != len(g.Treasures)-1 || len(finalGame.Equipment) != len(g.Equipment)-1 {
		t.Fatal("mixed sale changed capacity or removed an unselected instance")
	}
	for _, original := range g.Treasures {
		if original.GetGuid() == target.GetGuid() {
			continue
		}
		remaining, e := f.s.treasureRecord(finalGame, original.GetGuid())
		if e != nil || !proto.Equal(remaining, original) {
			t.Fatal("mixed sale changed an unselected/orange/red original022 reward", e)
		}
	}
	if _, err := f.s.treasureRecord(finalGame, target.GetGuid()); err == nil {
		t.Fatal("treasure GUID survived original sale")
	}
	if _, err := f.s.equipmentRecord(finalGame, equipment.GetGuid()); err == nil {
		t.Fatal("equipment GUID survived original sale")
	}
	if _, err := f.s.treasureBoxUse(requestContext(use), f.id, use, 93002); err == nil {
		t.Fatal("old original022 award receipt resurrected sold treasure")
	}
	reject(request, 93007)
	f.restart(t)
	replay, err := f.s.p5EquipmentSell(requestContext(request), f.id, request, 93006)
	if err != nil || !proto.Equal(replay[0], sold) {
		t.Fatal("cold original mixed-sale receipt", err)
	}
	cold, coldGame := f.profile(t)
	before, _ := json.Marshal([]any{final, finalGame})
	after, _ := json.Marshal([]any{cold, coldGame})
	if string(before) != string(after) {
		t.Fatal("cold sale replay duplicated coins or restored sold GUID")
	}
	if _, err := f.s.treasureBoxUse(requestContext(use), f.id, use, 93002); err == nil {
		t.Fatal("cold old source award restored retired GUID")
	}
}

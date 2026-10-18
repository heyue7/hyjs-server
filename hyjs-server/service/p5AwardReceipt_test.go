package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"testing"
)

func TestP5AwardReceiptOriginalBoxGrowthSaleAndSimpleReplay(t *testing.T) {
	f := p1Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.s.P5Ready = true
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(10000000)
		p.Diamond = proto.Int64(1000)
		_, e := f.s.GrantFixedReward(p, g, "item_067,1;item_051,1;hero_exp_001,1;item_041,1")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	requestCtx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	warUse := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_067"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(1)}
	warOut, e := f.s.p5BoxUse(requestCtx(warUse), f.id, warUse, 100)
	if e != nil {
		t.Fatal(e)
	}
	warReply := warOut[0].(*pb.SCItemUse)
	guid := warReply.GetGet().GetWars()[0].GetGuid()
	if out, e := f.s.p5BoxUse(requestCtx(warUse), f.id, warUse, 100); e != nil || !proto.Equal(out[0], warReply) {
		t.Fatal("unchanged war box receipt", e)
	}
	_, g := f.profile(t)
	var material string
	for _, w := range g.Warriors {
		if w.GetId() == "hero_exp_001" {
			material = w.GetGuid()
		}
	}
	grow := &pb.CSWarriorStrengthen{Guid: proto.String(guid), Cost: []*pb.BaseResource{{Id: proto.String("hero_exp_001"), Guid: proto.String(material), Num: proto.Int32(1)}}}
	if _, e = f.s.warriorStrengthen(requestCtx(grow), f.id, grow); e != nil {
		t.Fatal("actual awarded war growth", e)
	}
	if _, e = f.s.p5BoxUse(requestCtx(warUse), f.id, warUse, 100); e == nil {
		t.Fatal("old awarded war body accepted after growth")
	}
	f.restart(t)
	if _, e = f.s.p5BoxUse(requestCtx(warUse), f.id, warUse, 100); e == nil {
		t.Fatal("old war body cold accepted")
	}
	sellWar := &pb.CSWarriorSell{Guid: []string{guid}}
	if _, e = f.s.p5WarriorRecycle(requestCtx(sellWar), f.id, sellWar.Guid, false, 101, sellWar); e != nil {
		t.Fatal("actual awarded war sale", e)
	}
	if _, e = f.s.p5BoxUse(requestCtx(warUse), f.id, warUse, 100); e == nil {
		t.Fatal("retired war revived by old receipt")
	}
	equipUse := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_051"), Num: proto.Int32(1)}}
	equipOut, e := f.s.p5BoxUse(requestCtx(equipUse), f.id, equipUse, 200)
	if e != nil {
		t.Fatal(e)
	}
	equipReply := equipOut[0].(*pb.SCItemUse)
	equipment := equipReply.GetGet().GetEquips()[0]
	_, g = f.profile(t)
	if e = f.s.validateP5GrowthReceipt(g, &pb.SCCopyBox{CurrentResource: []*pb.BaseResource{{Id: equipment.EquipId, Guid: equipment.Guid, Num: proto.Int32(1)}}}); e != nil {
		t.Fatal("held copy descriptor", e)
	}
	quality := f.s.table("Equip", "id", equipment.GetEquipId())["quality"]
	costRow := f.s.table("EquipExp", "quality", quality)
	for _, row := range f.s.Tables["EquipExp"] {
		if row["quality"] == quality && row["level"] == "1" {
			costRow = row
			break
		}
	}
	coin, e := strconv.ParseInt(costRow["coin"], 10, 32)
	if e != nil {
		t.Fatal(e)
	}
	strength := &pb.CSEquipStrengthen{Guid: equipment.Guid, Times: proto.Int32(1), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(coin))}}}
	if _, e = f.s.equipmentStrengthenStamped(requestCtx(strength), f.id, strength, 201); e != nil {
		t.Fatal("actual box gear growth", e)
	}
	if _, e = f.s.p5BoxUse(requestCtx(equipUse), f.id, equipUse, 200); e == nil {
		t.Fatal("old gear body accepted after growth")
	}
	_, g = f.profile(t)
	if e = f.s.validateP5GrowthReceipt(g, &pb.SCCopyBox{CurrentResource: []*pb.BaseResource{{Id: equipment.EquipId, Guid: equipment.Guid, Num: proto.Int32(1)}}}); e != nil {
		t.Fatal("growth must preserve ID-only box descriptor", e)
	}
	sellEquip := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{{Id: equipment.EquipId, Guid: equipment.Guid, Num: proto.Int32(1)}}}
	if _, e = f.s.p5EquipmentSell(requestCtx(sellEquip), f.id, sellEquip, 202); e != nil {
		t.Fatal("actual box gear sale", e)
	}
	_, g = f.profile(t)
	if _, e = f.s.p5BoxUse(requestCtx(equipUse), f.id, equipUse, 200); e == nil {
		t.Fatal("retired gear revived")
	}
	if e = f.s.validateP5GrowthReceipt(g, &pb.SCCopyBox{CurrentResource: []*pb.BaseResource{{Id: equipment.EquipId, Guid: equipment.Guid, Num: proto.Int32(1)}}}); e == nil {
		t.Fatal("retired copy descriptor accepted")
	}
	if e = f.s.validateP5GrowthReceipt(g, &pb.SCItemUse{Get: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(100)}}}}); e != nil {
		t.Fatal("simple-only reward receipt must survive consumption", e)
	}
	if e = f.s.validateP5GrowthReceipt(g, &pb.SCCopyBox{CurrentResource: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(100)}}}); e != nil {
		t.Fatal("simple-only descriptor must remain replayable", e)
	}
	simpleUse := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_041"), Num: proto.Int32(1)}}
	simpleOut, e := f.s.p5BoxUse(requestCtx(simpleUse), f.id, simpleUse, 300)
	if e != nil {
		t.Fatal("actual simple-only original box", e)
	}
	if e = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Stamina = proto.Int32(10); return nil }); e != nil {
		t.Fatal(e)
	}
	consume := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_001"), Num: proto.Int32(1)}}
	if _, e = f.s.p5BasicItemUse(requestCtx(consume), f.id, consume, 301); e != nil {
		t.Fatal("actual simple reward consumption", e)
	}
	if out, e := f.s.p5BoxUse(requestCtx(simpleUse), f.id, simpleUse, 300); e != nil || !proto.Equal(out[0], simpleOut[0]) {
		t.Fatal("consumed simple box old receipt", e)
	}
	_, g = f.profile(t)
	// The same full-instance guard covers treasure awards without assuming an
	// original box exists for this fixture. Current unique identity is required.
	treasure := &pb.Treasure{Guid: proto.String("award-test-treasure"), TreasureId: proto.String("trea_001"), StrengthLevel: proto.Int32(1)}
	g.Treasures = append(g.Treasures, treasure)
	award := &pb.SCItemUse{Get: &pb.ResultObject{Treas: []*pb.Treasure{proto.Clone(treasure).(*pb.Treasure)}}}
	if e = f.s.validateP5GrowthReceipt(g, award); e != nil {
		t.Fatal(e)
	}
	treasure.StrengthLevel = proto.Int32(2)
	if e = f.s.validateP5GrowthReceipt(g, award); e == nil {
		t.Fatal("old treasure body after growth")
	}
}

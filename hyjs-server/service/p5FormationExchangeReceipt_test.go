package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP5FormationExchangeReceiptActualEquipmentGuardianAndCold(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitializeP9Pets, f.s.InitializeP9PetNext} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	f.s.P9PetsReady = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		_, err := f.s.GrantFixedReward(p, g, "hero_011,1;equip_037,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	_, g := f.profile(t)
	var hero *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_011" {
			hero = w
		}
	}
	gear := g.Equipment[0]
	heroOn := &pb.CSFormationOn{Id: hero.Id, Guid: hero.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.p5FormationOnNative(ctx(heroOn), f.id, heroOn, 196000); err != nil {
		t.Fatal(err)
	}
	equipOn := &pb.CSFormationOn{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(1)}
	if _, err := f.s.equipmentFormationOn(ctx(equipOn), f.id, equipOn); err != nil {
		t.Fatal(err)
	}
	firstRequest := &pb.CSPetFirstRecruit{}
	first, err := f.s.p9PetFirst(ctx(firstRequest), f.id, 196001, firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	pet := first[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	petOn := &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}
	if _, err := f.s.p9GuardOn(ctx(petOn), f.id, petOn, 196002); err != nil {
		t.Fatal(err)
	}
	exchange := &pb.CSFormationExChange{FromPos: proto.Int32(1), ToPos: proto.Int32(2)}
	out, err := f.s.formationExchange(ctx(exchange), f.id, exchange, 196003)
	if err != nil {
		t.Fatal(err)
	}
	receipt := out[0]
	f.restart(t)
	if again, err := f.s.formationExchange(ctx(exchange), f.id, exchange, 196003); err != nil || !proto.Equal(receipt, again[0]) {
		t.Fatal("current whole exchange cold", err)
	}
	equipOff := &pb.CSFormationOff{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.equipmentFormationOff(ctx(equipOff), f.id, equipOff); err != nil {
		t.Fatal(err)
	}
	reject := func() {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, err := f.s.formationExchange(ctx(exchange), f.id, exchange, 196003); err == nil {
			t.Fatal("old exchange rebuilt removed equipment or guardian descriptors")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("old exchange modified profile")
		}
	}
	reject()
	f.restart(t)
	reject()
	petOff := &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.p9GuardOff(ctx(petOff), f.id, petOff, 196004); err != nil {
		t.Fatal(err)
	}
	sell := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{{Id: gear.EquipId, Guid: gear.Guid, Num: proto.Int32(1)}}}
	if _, err := f.s.p5EquipmentSell(ctx(sell), f.id, sell, 196005); err != nil {
		t.Fatal(err)
	}
	reject()
	f.restart(t)
	reject()
	current, err := f.s.formationExchange(ctx(exchange), f.id, exchange, 196006)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if again, err := f.s.formationExchange(ctx(exchange), f.id, exchange, 196006); err != nil || !proto.Equal(current[0], again[0]) {
		t.Fatal("new clean exchange current cold", err)
	}
	// A broken in-memory ownership model must not pass merely because its slot
	// descriptors equal the receipt. No stored profile or real GUID is removed.
	_, g = f.profile(t)
	damaged := *g
	damaged.Warriors = nil
	if err := f.s.validateP5FormationExchangeReceipt(db.GetIns(), f.id, &damaged, current[0].(*pb.SCFormationExChange)); err == nil {
		t.Fatal("matching descriptor list without actual ownership accepted")
	}
}

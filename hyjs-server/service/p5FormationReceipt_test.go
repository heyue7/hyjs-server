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

// The finite original hero/equipment holdings isolate the current-slot receipt
// invariant. All replacement, unmount and destruction use real operations.
func TestP5FormationReceiptActualReplacementEquipmentSaleAndCold(t *testing.T) {
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
		_, err := f.s.GrantFixedReward(p, g, "hero_020,1;equip_001,1;equip_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	main := &pb.CSFormationOn{Id: proto.String("hero_043"), Guid: p.HeroWarId, FormatPos: proto.Int32(1)}
	var hero *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_020" {
			hero = w
		}
	}
	if hero == nil || len(g.Equipment) != 2 {
		t.Fatal("finite holdings missing")
	}
	selected := &pb.CSFormationOn{Id: hero.Id, Guid: hero.Guid, FormatPos: proto.Int32(1)}
	firstEquip, secondEquip := g.Equipment[0], g.Equipment[1]
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	on := func(b *pb.CSFormationOn, stamp int64) proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP5Next(ctx(b), f.id, b, stamp)
		if err != nil || !handled {
			t.Fatalf("actual formation: %v %v", handled, err)
		}
		return out[0]
	}
	checkCurrent := func(b *pb.CSFormationOn, stamp int64, original proto.Message) {
		t.Helper()
		if out := on(b, stamp); !proto.Equal(out, original) {
			t.Fatal("current receipt changed")
		}
	}
	reject := func(b *pb.CSFormationOn, stamp int64) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, _, err := f.s.DispatchP5Next(ctx(b), f.id, b, stamp); err == nil {
			t.Fatal("stale whole formation restored old slot")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("rejected receipt changed profile")
		}
	}
	putEquip := func(e *pb.Equipment) {
		t.Helper()
		if _, err := f.s.equipmentFormationOn(ctx(selected), f.id, &pb.CSFormationOn{Id: e.EquipId, Guid: e.Guid, FormatPos: proto.Int32(1)}); err != nil {
			t.Fatal(err)
		}
	}
	putEquip(firstEquip)
	first := on(selected, 191000)
	checkCurrent(selected, 191000, first)
	f.restart(t)
	checkCurrent(selected, 191000, first)
	on(main, 191001)
	reject(selected, 191000)
	f.restart(t)
	reject(selected, 191000)
	second := on(selected, 191002)
	checkCurrent(selected, 191002, second)
	putEquip(secondEquip)
	reject(selected, 191002)
	third := on(selected, 191003)
	checkCurrent(selected, 191003, third)
	off := &pb.CSFormationOff{Id: secondEquip.EquipId, Guid: secondEquip.Guid, FormatPos: proto.Int32(1)}
	if _, err := f.s.equipmentFormationOff(ctx(off), f.id, off); err != nil {
		t.Fatal(err)
	}
	reject(selected, 191003)
	sell := &pb.CSEquipmentSell{Cost: []*pb.BaseResource{{Id: secondEquip.EquipId, Guid: secondEquip.Guid, Num: proto.Int32(1)}}}
	if _, err := f.s.p5EquipmentSell(ctx(sell), f.id, sell, 191004); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	reject(selected, 191003)
	_, g = f.profile(t)
	if _, err := f.s.equipmentRecord(g, secondEquip.GetGuid()); err == nil {
		t.Fatal("sold equipment returned")
	}
	last := on(selected, 191005)
	f.restart(t)
	checkCurrent(selected, 191005, last)
}

func TestP5FormationReceiptRejectsDuplicateSlotAndChangedPower(t *testing.T) {
	slot := &pb.FormationItem{FormatPos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String("hero_020"), Guid: proto.String("original-held-guid")}}}
	p := &pb.Player{FightPower: proto.Int32(100)}
	g := &model.GameState{Formation: []*pb.FormationItem{proto.Clone(slot).(*pb.FormationItem)}}
	r := &pb.SCFormationOn{FormationItem: slot, Fightpower: proto.Int32(100)}
	if err := validateP5FormationReceipt(p, g, r); err != nil {
		t.Fatal(err)
	}
	p.FightPower = proto.Int32(101)
	if err := validateP5FormationReceipt(p, g, r); err == nil {
		t.Fatal("old derived power accepted")
	}
	p.FightPower = proto.Int32(100)
	g.Formation = append(g.Formation, proto.Clone(slot).(*pb.FormationItem))
	if err := validateP5FormationReceipt(p, g, r); err == nil {
		t.Fatal("ambiguous current slot accepted")
	}
	if err := validateP5FormationReceipt(p, g, &pb.SCFormationOn{}); err == nil {
		t.Fatal("nil slot accepted")
	}
	if err := validateP5FormationReceipt(p, g, &pb.SCItemUse{}); err != nil {
		t.Fatal("unrelated simple receipt changed")
	}
}

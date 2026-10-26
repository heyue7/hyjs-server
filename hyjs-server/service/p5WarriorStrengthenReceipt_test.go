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

func TestP5WarriorStrengthenCommittedReceiptOrderGrowthResetSoldAndCold(t *testing.T) {
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
		_, err := f.s.GrantFixedReward(p, g, "coin,800;diamond,50;hero_011,1;hero_exp_001,2")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	_, g := f.profile(t)
	var target *pb.Warrior
	var foods []*pb.Warrior
	for _, v := range g.Warriors {
		if v.GetId() == "hero_011" {
			target = v
		} else if v.GetId() == "hero_exp_001" {
			foods = append(foods, v)
		}
	}
	call := func(b *pb.CSWarriorStrengthen, stamp int64) ([]proto.Message, error) {
		out, handled, err := f.s.DispatchP5Next(ctx(b), f.id, b, stamp)
		if handled {
			return out, err
		}
		return f.s.warriorStrengthen(ctx(b), f.id, b)
	}
	keep := func(b *pb.CSWarriorStrengthen, stamp int64, want proto.Message) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		raw, _ := json.Marshal(beforeG)
		out, err := call(b, stamp)
		if err != nil || !proto.Equal(out[0], want) {
			t.Fatal("committed warrior strengthen did not replay current response", err)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(raw) != string(after) {
			t.Fatal("warrior retry consumed resources again")
		}
	}
	reject := func(b *pb.CSWarriorStrengthen, stamp int64) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		raw, _ := json.Marshal(beforeG)
		if _, err := call(b, stamp); err == nil {
			t.Fatal("old warrior growth body accepted")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(raw) != string(after) {
			t.Fatal("old warrior body changed profile")
		}
	}
	// Original upgrade packets omit num, so zero remains an accepted one-GUID material.
	first := &pb.CSWarriorStrengthen{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: foods[0].Id, Guid: foods[0].Guid}}}
	out, err := call(first, 199001)
	if err != nil {
		t.Fatal(err)
	}
	receipt := out[0]
	if receipt.(*pb.SCWarriorStrengthen).GetGuid().GetLevel() != 2 || receipt.(*pb.SCWarriorStrengthen).GetGuid().GetExp() != 260 {
		t.Fatal("original 400 experience boundary")
	}
	keep(first, 199001, receipt)
	f.restart(t)
	keep(first, 199001, receipt)
	on := &pb.CSFormationOn{Id: target.Id, Guid: target.Guid, FormatPos: proto.Int32(2)}
	if _, err = f.s.p5FormationOnNative(ctx(on), f.id, on, 199002); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	keep(first, 199001, receipt)
	row := f.s.table("Hero", "id", target.GetId())
	order := &pb.CSSetSkillOrder{HeroGuid: target.Guid, SkillOrder: proto.String(row["SkillId2"] + "," + row["SkillId1"])}
	if _, err = f.s.p5SkillOrder(ctx(order), f.id, order, 199003); err != nil {
		t.Fatal(err)
	}
	reject(first, 199001)
	f.restart(t)
	reject(first, 199001)
	second := &pb.CSWarriorStrengthen{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: foods[1].Id, Guid: foods[1].Guid}}}
	grown, err := call(second, 199004)
	if err != nil {
		t.Fatal(err)
	}
	reject(first, 199001)
	f.restart(t)
	keep(second, 199004, grown[0])
	off := &pb.CSFormationOff{Id: target.Id, Guid: target.Guid, FormatPos: proto.Int32(2)}
	if _, err = f.s.formationOff(ctx(off), f.id, off); err != nil {
		t.Fatal(err)
	}
	reset := &pb.CSWarriorReset{Guid: target.Guid}
	if _, err = f.s.p5WarriorReset(ctx(reset), f.id, reset, 199006); err != nil {
		t.Fatal(err)
	}
	reject(second, 199004)
	f.restart(t)
	reject(second, 199004)
	sale := &pb.CSWarriorSell{Guid: []string{target.GetGuid()}}
	if _, err = f.s.p5WarriorRecycle(ctx(sale), f.id, sale.Guid, false, 199007, sale); err == nil {
		t.Fatal("reset retired target GUID was sold twice")
	}
	reject(first, 199001)
	reject(second, 199004)
	f.restart(t)
	reject(second, 199004)
	p, g := f.profile(t)
	if p.GetCoin() != 800 || p.GetDiamond() != 0 || simpleOwned(p, g, "frag_hero_011") != 80 {
		t.Fatal("real growth/refund/sale conservation", p.GetCoin(), p.GetDiamond())
	}
	count := 0
	for _, v := range g.Warriors {
		if v.GetGuid() == target.GetGuid() {
			t.Fatal("sold warrior revived")
		}
		if v.GetId() == "hero_exp_001" {
			count++
		}
	}
	if count != 0 {
		t.Fatal("800 paidEXP is below5000 refund unit and must not recreate400-EXP donors", count)
	}
}

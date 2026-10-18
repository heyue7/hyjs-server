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

func TestP5TreasureStrengthenCommittedReceiptGrowthResetSoldAndCold(t *testing.T) {
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
		_, err := f.s.GrantFixedReward(p, g, "coin,40000;diamond,50;hero_011,1;trea_011,1;trea_013,2")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	_, g := f.profile(t)
	var target *pb.Treasure
	var foods []*pb.Treasure
	var hero *pb.Warrior
	for _, v := range g.Treasures {
		if v.GetTreasureId() == "trea_011" {
			target = v
		} else if v.GetTreasureId() == "trea_013" {
			foods = append(foods, v)
		}
	}
	for _, v := range g.Warriors {
		if v.GetId() == "hero_011" {
			hero = v
		}
	}
	onHero := &pb.CSFormationOn{Id: hero.Id, Guid: hero.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.p5FormationOnNative(ctx(onHero), f.id, onHero, 198000); err != nil {
		t.Fatal(err)
	}
	call := func(b *pb.CSTreasureStrengthen, stamp int64) ([]proto.Message, error) {
		out, handled, err := f.s.DispatchP5Next(ctx(b), f.id, b, stamp)
		if handled {
			return out, err
		}
		return f.s.treasureStrengthen(ctx(b), f.id, b)
	}
	keep := func(b *pb.CSTreasureStrengthen, stamp int64, want proto.Message) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		raw, _ := json.Marshal(beforeG)
		out, err := call(b, stamp)
		if err != nil || !proto.Equal(out[0], want) {
			t.Fatal("committed treasure strengthen did not replay current response", err)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(raw) != string(after) {
			t.Fatal("treasure retry consumed resources again")
		}
	}
	reject := func(b *pb.CSTreasureStrengthen, stamp int64) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		raw, _ := json.Marshal(beforeG)
		if _, err := call(b, stamp); err == nil {
			t.Fatal("old treasure growth body accepted")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(raw) != string(after) {
			t.Fatal("old treasure body changed profile")
		}
	}
	first := &pb.CSTreasureStrengthen{TargetId: target.Guid, Guids: []string{foods[0].GetGuid()}}
	out, err := call(first, 198001)
	if err != nil {
		t.Fatal(err)
	}
	receipt := out[0]
	if receipt.(*pb.SCTreasureStrengthen).GetTreasure().GetStrengthLevel() <= 1 {
		t.Fatal("actual original treasure growth missing")
	}
	keep(first, 198001, receipt)
	f.restart(t)
	keep(first, 198001, receipt)
	on := &pb.CSFormationOn{Id: target.TreasureId, Guid: target.Guid, FormatPos: proto.Int32(1)}
	if _, err = f.s.p5TreasureFormationOn(ctx(on), f.id, on, 198002); err != nil {
		t.Fatal(err)
	}
	off := &pb.CSFormationOff{Id: target.TreasureId, Guid: target.Guid, FormatPos: proto.Int32(1)}
	if _, err = f.s.p5TreasureFormationOff(ctx(off), f.id, off, 198003); err != nil {
		t.Fatal(err)
	}
	on.FormatPos = proto.Int32(2)
	if _, err = f.s.p5TreasureFormationOn(ctx(on), f.id, on, 198004); err != nil {
		t.Fatal(err)
	}
	// Original growth response updates the same Treasure dictionary, with no owner field.
	// A normal current equip relocation therefore keeps the current body replay valid.
	f.restart(t)
	keep(first, 198001, receipt)
	second := &pb.CSTreasureStrengthen{TargetId: target.Guid, Guids: []string{foods[1].GetGuid()}}
	grown, err := call(second, 198005)
	if err != nil {
		t.Fatal(err)
	}
	reject(first, 198001)
	f.restart(t)
	keep(second, 198005, grown[0])
	reject(first, 198001)
	off.FormatPos = proto.Int32(2)
	if _, err = f.s.p5TreasureFormationOff(ctx(off), f.id, off, 198006); err != nil {
		t.Fatal(err)
	}
	reset := &pb.CStreasureRebuild{TargetId: target.Guid}
	if _, err = f.s.p5TreasureReset(ctx(reset), f.id, reset, 198007); err != nil {
		t.Fatal(err)
	}
	reject(second, 198005)
	f.restart(t)
	reject(second, 198005)
	sale := &pb.CSTreasureSell{Guids: []string{target.GetGuid()}}
	if _, err = f.s.p5TreasureSell(ctx(sale), f.id, sale, 198008); err != nil {
		t.Fatal(err)
	}
	reject(first, 198001)
	reject(second, 198005)
	f.restart(t)
	reject(second, 198005)
	p, g := f.profile(t)
	if p.GetCoin() != 41000 || p.GetDiamond() != 0 {
		t.Fatal("real growth/refund/sale conservation", p.GetCoin(), p.GetDiamond())
	}
	count := 0
	for _, v := range g.Treasures {
		if v.GetGuid() == target.GetGuid() {
			t.Fatal("sold treasure revived")
		}
		if v.GetTreasureId() == "trea_013" {
			count++
		}
	}
	if count != 2 {
		t.Fatal("actual consumed treasure refund count", count)
	}
}

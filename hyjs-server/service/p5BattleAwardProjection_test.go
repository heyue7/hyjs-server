package service

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This test exercises the owned projection guard. Root owns the field/daily
// settled entry hooks; their transport lifecycle remains a separate check.
// The daily reward producer uses an explicit isolated reward contract fixture.
func TestP5BattleAwardProjectionRealProducerSoldAndCold(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy, f.s.InitializeP8Gather, f.s.InitializeP8Life, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(18)
		p.WarriorBagSize = proto.Int32(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	starts, err := f.s.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: g.Field.Monsters[0].Uuid})
	if err != nil {
		t.Fatal(err)
	}
	start := starts[0].(*pb.SCSidePveFightStart)
	party := proto.Clone(start.GetBattlePlayer()).(*pb.BattlePlayer)
	var hp float64
	for _, hero := range party.Formation {
		hero.Inst.CurHp = proto.Float64(hero.GetProp().GetHp())
		hp += hero.GetProp().GetHp()
	}
	ends, err := f.s.fieldFightEnd(f.ctx, f.id, &pb.CSSidePveFightEnd{FightId: start.FightId, BattlePlayer: party, Result: proto.Int32(1), OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(hp), DefenderTotalLeftHp: proto.Float64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	side := ends[0].(*pb.SCSidePveFightEnd)
	if len(side.GetGetn().GetWars()) < 1 {
		t.Fatal("actual original field material source missing")
	}
	var daily *pb.SCDailyCopyFightEnd
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		gain, err := f.s.dailyCopyGain(p, g, "hero_exp_006,1")
		daily = &pb.SCDailyCopyFightEnd{Get: gain}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	check := func(response proto.Message, want bool) {
		t.Helper()
		_, g := f.profile(t)
		if err := f.s.validateP5GrowthReceipt(g, response); (err == nil) != want {
			t.Fatal("original battle award projection", want, err)
		}
	}
	f.restart(t)
	check(side, true)
	check(daily, true)
	materials := []*pb.Warrior{side.GetGetn().GetWars()[0], daily.GetGet().GetWars()[0]}
	for index, material := range materials {
		request := &pb.CSWarriorSell{Guid: []string{material.GetGuid()}}
		ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
		if _, err := f.s.p5WarriorRecycle(ctx, f.id, request.Guid, false, int64(186000+index), request); err != nil {
			t.Fatal("actual original battle material sell", err)
		}
	}
	check(side, false)
	check(daily, false)
	f.restart(t)
	check(side, false)
	check(daily, false)
	check(&pb.SCSidePveFightEnd{Getn: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(100)}}}}, true)
	check(&pb.SCDailyCopyFightEnd{Get: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(100)}}}}, true)
}

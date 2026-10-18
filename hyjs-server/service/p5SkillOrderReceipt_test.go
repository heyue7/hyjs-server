package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP5SkillOrderReceiptActualReorderSaleAndCold(t *testing.T) {
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
		_, err := f.s.GrantFixedReward(p, g, "hero_011,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	var hero *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_011" {
			hero = w
		}
	}
	if hero == nil {
		t.Fatal("finite original actor missing")
	}
	row := f.s.table("Hero", "id", hero.GetId())
	a := &pb.CSSetSkillOrder{HeroGuid: hero.Guid, SkillOrder: proto.String(row["SkillId1"] + "," + row["SkillId2"])}
	b := &pb.CSSetSkillOrder{HeroGuid: hero.Guid, SkillOrder: proto.String(row["SkillId2"] + "," + row["SkillId1"])}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	order := func(request *pb.CSSetSkillOrder, stamp int64) proto.Message {
		t.Helper()
		out, err := f.s.p5SkillOrder(ctx(request), f.id, request, stamp)
		if err != nil {
			t.Fatal("actual skill order", err)
		}
		return out[0]
	}
	reject := func(request *pb.CSSetSkillOrder, stamp int64) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, err := f.s.p5SkillOrder(ctx(request), f.id, request, stamp); err == nil {
			t.Fatal("stale empty skill-order acknowledgement accepted")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("old order modified profile")
		}
	}
	first := order(a, 193000)
	f.restart(t)
	if !proto.Equal(first, order(a, 193000)) {
		t.Fatal("current order cold changed")
	}
	second := order(b, 193001)
	reject(a, 193000)
	f.restart(t)
	reject(a, 193000)
	if !proto.Equal(second, order(b, 193001)) {
		t.Fatal("current reversed order changed")
	}
	_, g = f.profile(t)
	current, err := p5Warrior(g, hero.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	transport, _, err := f.s.P5BattleSkillTransport(current)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(transport, row["SkillId2"]+",") {
		t.Fatal("reversed order not in real battle transport", transport)
	}
	sell := &pb.CSWarriorSell{Guid: []string{hero.GetGuid()}}
	if _, err := f.s.p5WarriorRecycle(ctx(sell), f.id, sell.Guid, false, 193002, sell); err != nil {
		t.Fatal("actual ordered actor sale", err)
	}
	reject(b, 193001)
	f.restart(t)
	reject(b, 193001)
}

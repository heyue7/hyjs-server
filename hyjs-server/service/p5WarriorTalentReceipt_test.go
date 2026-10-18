package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This domain fixture supplies finite level/material prerequisites explicitly.
// It proves the original talent transaction and lost-response lifecycle, not
// natural resource acquisition or a new native combat family.
func TestP5WarriorTalentCommittedReceiptAwakeAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{
		f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeWarriorAwake,
		f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks,
	} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(70)
		p.Coin = proto.Int64(100000)
		p.WarriorBagSize = proto.Int32(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_003,3;item_005,180;item_322,800;item_315,50;item_312,15")
		if err != nil {
			return err
		}
		guid = gain.GetWars()[0].GetGuid()
		w, err := p5Warrior(g, guid)
		if err != nil {
			return err
		}
		w.Level = proto.Int32(70)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	stable := func() string {
		t.Helper()
		p, g := f.profile(t)
		raw, err := json.Marshal([]any{p, g})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	request := &pb.CSWarriorTalent{Guid: proto.String(guid), Talent: proto.String("1"),
		Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(10000)}, {Id: proto.String("item_005"), Num: proto.Int32(30)}}}
	beforeP, beforeG := f.profile(t)
	beforeWarrior, err := p5Warrior(beforeG, guid)
	if err != nil {
		t.Fatal(err)
	}
	beforeActor, err := f.s.battleWarrior(beforeP, beforeWarrior, beforeG, &pb.FormationItem{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.s.p5WarriorTalent(ctx(request), f.id, request, 201001)
	if err != nil {
		t.Fatal(err)
	}
	first := out[0].(*pb.SCWarriorTalent)
	afterP, afterG := f.profile(t)
	afterWarrior, err := p5Warrior(afterG, guid)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetGuid().GetTalent() != 1 || afterP.GetCoin() != 90000 || simpleOwned(afterP, afterG, "item_005") != 150 {
		t.Fatal("original node1 actual state/cost", first, afterP.GetCoin())
	}
	afterActor, err := f.s.battleWarrior(afterP, afterWarrior, afterG, &pb.FormationItem{})
	if err != nil {
		t.Fatal(err)
	}
	if afterActor.GetProp().GetAtt() <= beforeActor.GetProp().GetAtt() || afterActor.GetProp().GetHp() <= beforeActor.GetProp().GetHp() {
		t.Fatal("activated original talent did not reach battle actor")
	}
	keep := func() {
		t.Helper()
		saved := stable()
		retry, e := f.s.p5WarriorTalent(ctx(request), f.id, request, 201001)
		if e != nil || !proto.Equal(retry[0], first) || saved != stable() {
			t.Fatal("lost callback must replay without spending", e)
		}
	}
	keep()
	f.restart(t)
	keep()
	// Eight concurrent retries share the committed transaction; none spends again.
	saved := stable()
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			retry, e := f.s.p5WarriorTalent(ctx(request), f.id, request, 201001)
			if e != nil {
				failures <- e
				return
			}
			if !proto.Equal(retry[0], first) {
				failures <- p5Error()
			}
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Error("concurrent retry", e)
	}
	if stable() != saved {
		t.Fatal("replayed talent spent again")
	}
	reject := func(b *pb.CSWarriorTalent, stamp int64) {
		t.Helper()
		saved := stable()
		if _, e := f.s.p5WarriorTalent(ctx(b), f.id, b, stamp); e == nil {
			t.Fatal("invalid or retired talent callback accepted")
		}
		if stable() != saved {
			t.Fatal("rejected request changed saved profile")
		}
	}
	conflict := proto.Clone(request).(*pb.CSWarriorTalent)
	conflict.Talent = proto.String("2")
	reject(conflict, 201001)
	reject(request, 201000)
	reject(request, 0)
	// A real next growth operation changes the same actor. The historical talent
	// frame must then fail closed so its complete Warrior cannot undo awakening.
	awake := &pb.CSWarriorAwake{Guid: proto.String(guid)}
	awakened, err := f.s.warriorAwake(ctx(awake), f.id, awake, 201002)
	if err != nil || awakened[0].(*pb.SCWarriorAwake).GetNewWarrior().GetAwake().GetStage() != 1 {
		t.Fatal("actual awakening", err)
	}
	if _, e := f.s.p5WarriorTalent(ctx(request), f.id, request, 201001); e == nil || !strings.Contains(e.Error(), constUtil.P5GrowthReceiptChanged) {
		t.Fatal("later awakening did not retire complete old talent frame", e)
	}
	reject(request, 201001)
	f.restart(t)
	reject(request, 201001)
	// A fresh timestamp and the real next node remain playable after awakening.
	next := &pb.CSWarriorTalent{Guid: proto.String(guid), Talent: proto.String("2"),
		Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(20000)}, {Id: proto.String("item_005"), Num: proto.Int32(150)}}}
	second, err := f.s.p5WarriorTalent(ctx(next), f.id, next, 201003)
	if err != nil || second[0].(*pb.SCWarriorTalent).GetGuid().GetTalent() != 2 {
		t.Fatal("next node after awakening", err)
	}
	f.restart(t)
	p, g := f.profile(t)
	current, err := p5Warrior(g, guid)
	if err != nil || current.GetTalent() != 2 || current.GetAwake().GetStage() != 1 || p.GetCoin() != 70000 || simpleOwned(p, g, "item_005") != 0 || len(g.Warriors) != len(beforeG.Warriors)-2 {
		t.Fatal("cold state/cost/donor conservation", err)
	}
	retry, err := f.s.p5WarriorTalent(ctx(next), f.id, next, 201003)
	if err != nil || !proto.Equal(retry[0], second[0]) {
		t.Fatal("current node cold replay", err)
	}
	reject(request, 201001)
}

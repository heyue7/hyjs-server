package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestWarriorBreach84PityPrecisionAndBound(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int64
	}{{"0", 0}, {"0.002", 20}, {"0.079", 790}, {"0.485", 4850}, {"10.0", 100000}, {"100", 1000000}} {
		value, err := breachProbabilityUnits(tc.text)
		if err != nil || value != tc.want {
			t.Fatalf("%s: %d, %v", tc.text, value, err)
		}
	}
	for _, invalid := range []string{"NaN", "-0.01", "100.1", "0.00001", "broken"} {
		if _, err := breachProbabilityUnits(invalid); err == nil {
			t.Fatalf("invalid probability accepted: %s", invalid)
		}
	}
	row := map[string]string{"ExpIncrease": "2,100", "Critical1": "48", "Critical2": "67", "Critical3": "102", "Criprobability1": "0", "Criprobability2": "10", "Criprobability3": "20", "Criprobability4": "30"}
	for _, tc := range []struct {
		exp  int32
		want int64
	}{{47, 0}, {48, 100000}, {66, 100000}, {67, 200000}, {101, 200000}, {102, 300000}} {
		_, got, err := breachOptions(row, tc.exp)
		if err != nil || got != tc.want {
			t.Fatalf("at exp %d: %d, %v", tc.exp, got, err)
		}
	}
	for _, p := range []int64{20, 790, 4850, 100000, 1000000} {
		for _, initial := range []int64{0, 1, 999999} {
			g := &model.GameState{WarriorBreachPity: map[string]*model.WarriorBreachPity{"a": {Phase: initial}}}
			bound := (constUtil.WarriorBreachPityScale + p - 1) / p
			first := int64(0)
			for step := int64(1); step <= bound; step++ {
				hit, err := advanceWarriorBreachPity(g, "a", p, nil)
				if err != nil {
					t.Fatal(err)
				}
				if hit {
					first = step
					if g.WarriorBreachPity["a"].Phase != initial+step*p-constUtil.WarriorBreachPityScale {
						t.Fatal("remainder lost")
					}
					break
				}
			}
			if first == 0 || first > bound {
				t.Fatalf("p=%d phase=%d lacks %d-step bound", p, initial, bound)
			}
		}
	}
	g := &model.GameState{}
	if hit, err := advanceWarriorBreachPity(g, "a", 0, nil); err != nil || hit || len(g.WarriorBreachPity) != 0 {
		t.Fatal("zero probability initialized or advanced state")
	}
	if _, err := advanceWarriorBreachPity(g, "a", 20, io.LimitReader(bytes.NewReader(nil), 0)); err == nil || len(g.WarriorBreachPity) != 0 {
		t.Fatal("random failure mutated saved state")
	}
	if hit, err := advanceWarriorBreachPity(g, "a", 20, bytes.NewReader([]byte{0, 0, 0})); err != nil || hit || g.WarriorBreachPity["a"].Phase != 20 {
		t.Fatal("initial phase", err)
	}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var reopened model.GameState
	if err = json.Unmarshal(data, &reopened); err != nil || reopened.WarriorBreachPity["a"].Phase != 20 {
		t.Fatal("phase did not survive profile encoding", err)
	}
	reopened.WarriorBreachPity["a"] = nil
	if _, err = advanceWarriorBreachPity(&reopened, "a", 0, nil); err == nil {
		t.Fatal("corrupt saved phase silently reinitialized")
	}
}

func breach84Setup(t *testing.T) *p1Fixture {
	t.Helper()
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	return f
}

func breach84Context(f *p1Fixture, request proto.Message) context.Context {
	return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
}

func breach84JSON(t *testing.T, g *model.GameState) string {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The finite fixture exercises the real transaction and original receipt. A
// deliberately old timestamp and nonzero legacy expiry make a daily reset
// visible; a failed ten-step batch must roll back its first successful step.
func TestWarriorBreach84TransactionAndColdReceipt(t *testing.T) {
	f := breach84Setup(t)
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Diamond = proto.Int32(100), proto.Int64(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_011,1;item_006,2")
		if err != nil {
			return err
		}
		guid = gain.GetWars()[0].GetGuid()
		w, err := p5Warrior(g, guid)
		if err != nil {
			return err
		}
		w.Breached = &pb.Breached{Level: proto.Int32(1), Exp: proto.Int32(48), LastAddExpTime: proto.String("1")}
		g.WarriorBreachPity = map[string]*model.WarriorBreachPity{guid: {Phase: 950000}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", "breach").Updates(map[string]any{"reset_seconds": 1, "critical_multiplier": 0}).Error; err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := f.profile(t)
	batch := &pb.CSWarriorBreached{Guid: proto.String(guid), Type: proto.Int32(2)}
	if _, err := f.s.warriorBreached(breach84Context(f, batch), f.id, batch, 84001); err == nil {
		t.Fatal("batch with insufficient second-step materials succeeded")
	}
	afterP, afterG := f.profile(t)
	if !proto.Equal(beforeP, afterP) || breach84JSON(t, beforeG) != breach84JSON(t, afterG) {
		t.Fatal("failed batch advanced experience, resources, investment or pity")
	}
	single := &pb.CSWarriorBreached{Guid: proto.String(guid), Type: proto.Int32(1)}
	out, err := f.s.warriorBreached(breach84Context(f, single), f.id, single, 84001)
	if err != nil {
		t.Fatal(err)
	}
	result := out[0].(*pb.SCWarriorBreached)
	if result.GetGuid().GetBreached().GetLevel() != 2 || result.GetGuid().GetBreached().GetExp() != 0 || len(result.GetCost()) != 1 || result.GetCost()[0].GetId() != "item_006" || result.GetCost()[0].GetNum() != 2 {
		t.Fatal("old timestamp or multiplier changed guaranteed direct-level result", result)
	}
	paidP, paidG := f.profile(t)
	if paidG.WarriorBreachPity[guid].Phase != 50000 || simpleOwned(beforeP, beforeG, "item_006")-simpleOwned(paidP, paidG, "item_006") != 2 || p5KV(paidG, "p5.invest:"+guid) == "" {
		t.Fatal("committed phase, actual cost or investment missing")
	}
	f.restart(t)
	replay, err := f.s.warriorBreached(breach84Context(f, single), f.id, single, 84001)
	if err != nil || !proto.Equal(replay[0], out[0]) {
		t.Fatal("cold duplicate receipt", err)
	}
	replayedP, replayedG := f.profile(t)
	if !proto.Equal(paidP, replayedP) || breach84JSON(t, paidG) != breach84JSON(t, replayedG) {
		t.Fatal("cold replay advanced phase or cost")
	}
	reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
	if _, err := f.s.p5WarriorReset(breach84Context(f, reset), f.id, reset, 84002); err != nil {
		t.Fatal("reset after completed paid breakthrough", err)
	}
	resetP, resetG := f.profile(t)
	if _, ok := resetG.WarriorBreachPity[guid]; ok || simpleOwned(resetP, resetG, "item_006") != 2 {
		t.Fatal("reset did not retire pity or return completed-stage investment")
	}
}

// Finite entry-state fixtures cover ordinary carry, a ten-step request and
// early stopping on terminal 25 without requiring a full progression run.
func TestWarriorBreach84CarryAndTerminalCosts(t *testing.T) {
	f := breach84Setup(t)
	var natural, terminal string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_011,2;item_006,500")
		if err != nil {
			return err
		}
		natural, terminal = gain.GetWars()[0].GetGuid(), gain.GetWars()[1].GetGuid()
		for _, entry := range []struct {
			guid       string
			level, exp int32
		}{{natural, 1, 119}, {terminal, 24, 96899}} {
			w, e := p5Warrior(g, entry.guid)
			if e != nil {
				return e
			}
			w.Breached = &pb.Breached{Level: proto.Int32(entry.level), Exp: proto.Int32(entry.exp)}
		}
		g.WarriorBreachPity = map[string]*model.WarriorBreachPity{natural: {Phase: 0}, terminal: {Phase: 0}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	single := &pb.CSWarriorBreached{Guid: proto.String(natural), Type: proto.Int32(1)}
	out, err := f.s.warriorBreached(breach84Context(f, single), f.id, single, 84101)
	if err != nil {
		t.Fatal(err)
	}
	result := out[0].(*pb.SCWarriorBreached)
	if result.GetGuid().GetBreached().GetLevel() != 2 || result.GetGuid().GetBreached().GetExp() != 1 {
		t.Fatal("ordinary gain remainder changed", result)
	}
	_, state := f.profile(t)
	if state.WarriorBreachPity[natural].Phase != 300000 {
		t.Fatal("natural level-up lost pity")
	}
	ten := &pb.CSWarriorBreached{Guid: proto.String(natural), Type: proto.Int32(2)}
	out, err = f.s.warriorBreached(breach84Context(f, ten), f.id, ten, 84102)
	if err != nil {
		t.Fatal(err)
	}
	result = out[0].(*pb.SCWarriorBreached)
	if len(result.GetCost()) != 1 || result.GetCost()[0].GetNum() != 50 || result.GetGuid().GetBreached().GetLevel() != 2 {
		t.Fatal("ten original stage-two costs changed", result)
	}
	_, state = f.profile(t)
	if state.WarriorBreachPity[natural].Phase != 300000 {
		t.Fatal("zero-probability ten-step request changed phase")
	}
	last := &pb.CSWarriorBreached{Guid: proto.String(terminal), Type: proto.Int32(2)}
	out, err = f.s.warriorBreached(breach84Context(f, last), f.id, last, 84103)
	if err != nil {
		t.Fatal(err)
	}
	result = out[0].(*pb.SCWarriorBreached)
	if result.GetGuid().GetBreached().GetLevel() != 25 || result.GetGuid().GetBreached().GetExp() != 0 || len(result.GetCost()) != 1 || result.GetCost()[0].GetNum() != 30 {
		t.Fatal("terminal ten-step request charged extra steps", result)
	}
	_, state = f.profile(t)
	if state.WarriorBreachPity[terminal].Phase != 4850 {
		t.Fatal("terminal natural advance lost remainder")
	}
	if _, err = f.s.warriorBreached(breach84Context(f, last), f.id, last, 84104); err == nil {
		t.Fatal("maximum level accepted a new paid step")
	}
}

// A real partially breached donor transfers its recorded cost without
// transferring its probability phase; target reset uses the existing refund
// policy for completed versus incomplete breach investment.
func TestWarriorBreach84DonorInvestmentAndReset(t *testing.T) {
	f := breach84Setup(t)
	var target, donor string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(100), proto.Int64(100000), proto.Int64(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_011,2;item_006,100")
		if err != nil {
			return err
		}
		target, donor = gain.GetWars()[0].GetGuid(), gain.GetWars()[1].GetGuid()
		g.WarriorBreachPity = map[string]*model.WarriorBreachPity{target: {Phase: 123456}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		kind := int32(1)
		if i < 2 {
			kind = 2
		}
		req := &pb.CSWarriorBreached{Guid: proto.String(donor), Type: proto.Int32(kind)}
		if _, err := f.s.warriorBreached(breach84Context(f, req), f.id, req, int64(84201+i)); err != nil {
			t.Fatalf("donor paid step %d: %v", i, err)
		}
	}
	_, beforeG := f.profile(t)
	if beforeG.WarriorBreachPity[donor] == nil || beforeG.WarriorBreachPity[target].Phase != 123456 {
		t.Fatal("donor and target phases missing")
	}
	w, err := p5Warrior(beforeG, donor)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := p5InvestmentState(beforeG, donor, w)
	if err != nil {
		t.Fatal(err)
	}
	refund, err := f.s.p5WarriorRefundCosts(beforeG, ledger, w)
	if err != nil {
		t.Fatal(err)
	}
	wantCrystals := int64(0)
	for _, item := range refund {
		if item.GetId() == "item_006" {
			wantCrystals += int64(item.GetNum())
		}
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.lock:"+donor, "1")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	protectedP, protectedG := f.profile(t)
	feed := &pb.CSWarriorStrengthen{Guid: proto.String(target), Cost: []*pb.BaseResource{{Id: proto.String("hero_011"), Guid: proto.String(donor)}}}
	if _, err = f.s.p5WarriorStrengthen(breach84Context(f, feed), f.id, feed, 84220); err == nil {
		t.Fatal("locked donor accepted")
	}
	afterP, afterG := f.profile(t)
	if !proto.Equal(protectedP, afterP) || breach84JSON(t, protectedG) != breach84JSON(t, afterG) {
		t.Fatal("failed donor removal persisted a phase or investment change")
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.lock:"+donor, "")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p5WarriorStrengthen(breach84Context(f, feed), f.id, feed, 84220); err != nil {
		t.Fatal("paid donor transfer", err)
	}
	paidP, paidG := f.profile(t)
	if _, ok := paidG.WarriorBreachPity[donor]; ok || paidG.WarriorBreachPity[target].Phase != 123456 {
		t.Fatal("donor pity survived or target pity was inherited/changed")
	}
	current, err := p5Warrior(paidG, target)
	if err != nil {
		t.Fatal(err)
	}
	transferred, err := p5InvestmentState(paidG, target, current)
	if err != nil {
		t.Fatal(err)
	}
	var invested int32
	for _, item := range transferred.Costs {
		if item.GetId() == "item_006" {
			invested += item.GetNum()
		}
	}
	if invested != 50 {
		t.Fatal("donor actual breach costs were not inherited once", invested)
	}
	reset := &pb.CSWarriorReset{Guid: proto.String(target)}
	if _, err = f.s.p5WarriorReset(breach84Context(f, reset), f.id, reset, 84221); err != nil {
		t.Fatal("target reset", err)
	}
	resetP, resetG := f.profile(t)
	if _, ok := resetG.WarriorBreachPity[target]; ok || simpleOwned(resetP, resetG, "item_006")-simpleOwned(paidP, paidG, "item_006") != wantCrystals {
		t.Fatal("target pity or original donor refund policy changed", wantCrystals)
	}
}

func TestWarriorBreach84MainResetRetiresPity(t *testing.T) {
	f := breach84Setup(t)
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Diamond = proto.Int32(100), proto.Int64(100)
		guid = p.GetHeroWarId()
		_, err := f.s.GrantFixedReward(p, g, "item_006,100")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		kind := int32(1)
		if i < 2 {
			kind = 2
		}
		req := &pb.CSWarriorBreached{Guid: proto.String(guid), Type: proto.Int32(kind)}
		if _, err := f.s.warriorBreached(breach84Context(f, req), f.id, req, int64(84301+i)); err != nil {
			t.Fatalf("main paid step %d: %v", i, err)
		}
	}
	_, before := f.profile(t)
	if before.WarriorBreachPity[guid] == nil {
		t.Fatal("main never initialized pity")
	}
	reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
	out, err := f.s.p5WarriorReset(breach84Context(f, reset), f.id, reset, 84320)
	if err != nil {
		t.Fatal(err)
	}
	player, after := f.profile(t)
	main, err := p5Warrior(after, guid)
	if err != nil || player.GetHeroWarId() != guid || main.GetBreached().GetLevel() != 1 {
		t.Fatal("main identity or reset baseline changed", err)
	}
	if _, ok := after.WarriorBreachPity[guid]; ok {
		t.Fatal("refunded main retained old pity")
	}
	f.restart(t)
	replay, err := f.s.p5WarriorReset(breach84Context(f, reset), f.id, reset, 84320)
	if err != nil || !proto.Equal(out[0], replay[0]) {
		t.Fatal("main cold reset replay", err)
	}
	_, cold := f.profile(t)
	if breach84JSON(t, after) != breach84JSON(t, cold) {
		t.Fatal("main reset replay changed pity or investment")
	}
}

// The helper rejects absent or corrupt entropy before any state is installed.
type breach84ReaderFailure struct{}

func (breach84ReaderFailure) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestWarriorBreach84RandomFailure(t *testing.T) {
	g := &model.GameState{}
	if _, err := advanceWarriorBreachPity(g, "a", 100, breach84ReaderFailure{}); err == nil || len(g.WarriorBreachPity) != 0 {
		t.Fatal(fmt.Sprint("entropy failure persisted a phase: ", err))
	}
}

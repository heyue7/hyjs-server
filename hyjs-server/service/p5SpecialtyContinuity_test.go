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

// An exact before fingerprint is required. Free defaults may not create
// refundable resources, repair a forged old ledger or charge anything.
func TestP5SpecialtyContinuityPristineAndUntrusted(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	rule := &db.SkillSpecialtyRule{InitialTypes: "1,2,3"}
	w := &pb.Warrior{Id: proto.String("hero_003"), Guid: proto.String("pristine"), Level: proto.Int32(75), Breached: &pb.Breached{Level: proto.Int32(1)}}
	g := &model.GameState{Warriors: []*pb.Warrior{w}}
	if err := f.s.specialtyInitializeTracked(g, w, rule); err != nil || len(w.GetSkillRefine()) != 3 {
		t.Fatal("original free initialization", err)
	}
	if p5KV(g, "p5.invest:"+w.GetGuid()) != "" {
		t.Fatal("free defaults created refundable investment")
	}
	before := &pb.Warrior{Id: proto.String("hero_003"), Guid: proto.String("invested"), Level: proto.Int32(74), Breached: &pb.Breached{Level: proto.Int32(1)}}
	current := proto.Clone(before).(*pb.Warrior)
	current.Training = &pb.Training{Att: proto.Int32(1)}
	g = &model.GameState{Warriors: []*pb.Warrior{current}}
	if err := RecordP5Investment(g, current.GetGuid(), before, current, []*pb.BaseResource{{Id: proto.String("item_007"), Num: proto.Int32(3)}}); err != nil {
		t.Fatal(err)
	}
	old := p5KV(g, "p5.invest:"+current.GetGuid())
	current.Level = proto.Int32(75) // Explicitly untracked mutation; must not be laundered.
	if err := f.s.specialtyInitializeTracked(g, current, rule); err == nil {
		t.Fatal("untrusted before fingerprint accepted")
	}
	if p5KV(g, "p5.invest:"+current.GetGuid()) != old {
		t.Fatal("old ledger was rewritten on rejection")
	}
}

// Domain prerequisites are finite Player75/target74-near-threshold and exact
// inventory/currency. Actual training, original food consumption and specialty
// costs are exercised; natural acquisition and native battle are not claimed.
func TestP5SpecialtyContinuityCross75GrowthResetAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeWarriorAwake, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitSkillSpecialty} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	var guid, foodGUID string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Coin = proto.Int64(1000000)
		p.Diamond = proto.Int64(1000)
		gain, err := f.s.GrantFixedReward(p, g, "hero_003,1;hero_exp_001,1;item_007,6;item_439,20;item_440,15")
		if err != nil {
			return err
		}
		for _, card := range gain.GetWars() {
			if card.GetId() == "hero_003" {
				guid = card.GetGuid()
			} else if card.GetId() == "hero_exp_001" {
				foodGUID = card.GetGuid()
			}
		}
		w, err := p5Warrior(g, guid)
		if err != nil {
			return err
		}
		need, err := f.s.warriorLevelExperience("23", 74)
		if err != nil {
			return err
		}
		w.Level = proto.Int32(74)
		w.Exp = proto.Int64(need - 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	snapshot := func() string {
		t.Helper()
		p, g := f.profile(t)
		raw, err := json.Marshal([]any{p, g})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	p, g := f.profile(t)
	if _, err := p5Warrior(g, guid); err != nil {
		t.Fatal(err)
	}
	var stamp int64 = 203000
	train := func() {
		t.Helper()
		stamp++
		b := &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(3), NumType: proto.Int32(1)}
		out, err := f.s.warriorTraining(ctx(b), f.id, b, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if out[0].(*pb.SCWarriorTraining).GetGuid().GetTempTraining().GetState() != 1 {
			t.Fatal("original positive high training pending missing")
		}
		stamp++
		save := &pb.CSWarriorTrainingReplace{Guid: proto.String(guid), Result: proto.Int32(1)}
		if _, err := f.s.warriorTrainingReplace(ctx(save), f.id, save, stamp); err != nil {
			t.Fatal(err)
		}
	}
	train()
	_, g = f.profile(t)
	w, err := p5Warrior(g, guid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p5InvestmentState(g, guid, w); err != nil {
		t.Fatal("real pre-gate investment", err)
	}
	stamp++
	level := &pb.CSWarriorStrengthen{Guid: proto.String(guid), Cost: []*pb.BaseResource{{Id: proto.String("hero_exp_001"), Guid: proto.String(foodGUID)}}}
	out, err := f.s.p5WarriorStrengthen(ctx(level), f.id, level, stamp)
	if err != nil || out[0].(*pb.SCWarriorStrengthen).GetGuid().GetLevel() != 75 {
		t.Fatal("actual original level75 crossing", err)
	}
	p, g = f.profile(t)
	w, err = p5Warrior(g, guid)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := p5InvestmentState(g, guid, w)
	if err != nil {
		t.Fatal(err)
	}
	priorRaw := p5KV(g, "p5.invest:"+guid)
	priorCosts := append([]*pb.BaseResource(nil), prior.Costs...)
	// A damaged existing ledger must reject the preparation transaction and
	// leave both the complete profile and the original ledger unchanged.
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		var v p5Investment
		if err := json.Unmarshal([]byte(p5KV(g, "p5.invest:"+guid)), &v); err != nil {
			return err
		}
		v.Expected = "untrusted"
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		p5SetKV(g, "p5.invest:"+guid, string(raw))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	damaged := snapshot()
	if err := f.s.skillSpecialtyPrepare(f.ctx, f.id); err == nil {
		t.Fatal("prepare repaired an untrusted ledger")
	}
	if snapshot() != damaged {
		t.Fatal("failed preparation mutated profile")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { p5SetKV(g, "p5.invest:"+guid, priorRaw); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := f.s.skillSpecialtyPrepare(f.ctx, f.id); err != nil {
		t.Fatal("original list/login preparation", err)
	}
	p, g = f.profile(t)
	w, err = p5Warrior(g, guid)
	if err != nil {
		t.Fatal(err)
	}
	tracked, err := p5InvestmentState(g, guid, w)
	if err != nil || len(w.GetSkillRefine()) != 3 {
		t.Fatal("gate defaults broke investment fingerprint", err)
	}
	if string(tracked.Baseline) != string(prior.Baseline) || !equipmentResourcesEqual(tracked.Costs, priorCosts) {
		t.Fatal("free preparation changed refundable baseline or cost")
	}
	if err := f.s.skillSpecialtyPrepare(f.ctx, f.id); err != nil {
		t.Fatal("repeated preparation", err)
	}
	f.restart(t)
	p, g = f.profile(t)
	w, err = p5Warrior(g, guid)
	if err != nil {
		t.Fatal(err)
	}
	slot := &pb.FormationItem{}
	beforeActor, err := f.s.battleWarrior(p, w, g, slot)
	if err != nil {
		t.Fatal(err)
	}
	stamp++
	up := &pb.CSUpRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(1)}
	first, err := f.s.skillSpecialtyUpgrade(ctx(up), f.id, up, stamp)
	if err != nil {
		t.Fatal("continued real specialty upgrade", err)
	}
	replay, err := f.s.skillSpecialtyUpgrade(ctx(up), f.id, up, stamp)
	if err != nil || !proto.Equal(replay[0], first[0]) {
		t.Fatal("current specialty replay", err)
	}
	p, g = f.profile(t)
	w, err = p5Warrior(g, guid)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.battleWarrior(p, w, g, slot)
	if err != nil || actor.GetProp().GetHp() != beforeActor.GetProp().GetHp()+500 {
		t.Fatal("original level2 HP500 not in actual battle actor", err)
	}
	train() // Continued growth must still use the same trusted investment.
	stamp++
	reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
	done, err := f.s.p5WarriorReset(ctx(reset), f.id, reset, stamp)
	if err != nil {
		t.Fatal("reset after cross-gate specialty/training", err)
	}
	f.restart(t)
	p, g = f.profile(t)
	w, err = p5Warrior(g, guid)
	if err == nil || simpleOwned(p, g, "frag_hero_003") != 150 {
		t.Fatal("cold reset must retire partnerGUID and return150 original fragments", err)
	}
	if p.GetCoin() != 1000000 || p.GetDiamond() != 940 || simpleOwned(p, g, "item_007") != 6 || simpleOwned(p, g, "item_439") != 20 || simpleOwned(p, g, "item_440") != 15 {
		t.Fatal("refundable materials must return; two training diamond fees and reset fee remain")
	}
	foods := 0
	for _, card := range g.Warriors {
		if card.GetId() == "hero_exp_001" {
			foods++
			if card.GetGuid() == foodGUID {
				t.Fatal("consumed GUID resurrected")
			}
		}
	}
	if foods != 0 {
		t.Fatal("400 paidEXP below5000 cannot recreate original donor", foods)
	}
	current := snapshot()
	again, err := f.s.p5WarriorReset(ctx(reset), f.id, reset, stamp)
	if err != nil || !proto.Equal(again[0], done[0]) || snapshot() != current {
		t.Fatal("reset cold replay granted or charged twice", err)
	}
	if err := f.s.skillSpecialtyPrepare(f.ctx, f.id); err != nil {
		t.Fatal("removed partner not eligible", err)
	}
	_, g = f.profile(t)
	if p5KV(g, "p5.invest:"+guid) != "" {
		t.Fatal("free prepare recreated cleared ledger")
	}
}

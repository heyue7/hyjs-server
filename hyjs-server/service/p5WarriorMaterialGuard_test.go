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

func TestP5WarriorMaterialOwnershipAndPending(t *testing.T) {
	cases := []struct {
		name    string
		protect func(*pb.Player, *model.GameState, *pb.Warrior, map[string]bool)
	}{
		{"main", func(p *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) { p.HeroWarId = w.Guid }},
		{"formation-any-slot", func(_ *pb.Player, g *model.GameState, w *pb.Warrior, _ map[string]bool) {
			g.Formation = []*pb.FormationItem{{FormatPos: proto.Int32(6), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: w.Guid, Id: w.Id}}}}
		}},
		{"helper", func(_ *pb.Player, g *model.GameState, w *pb.Warrior, _ map[string]bool) {
			p5SetKV(g, "p5.helper:6", w.GetGuid())
		}},
		{"locked", func(_ *pb.Player, g *model.GameState, w *pb.Warrior, _ map[string]bool) {
			p5SetKV(g, "p5.lock:"+w.GetGuid(), "1")
		}},
		{"dispatch-kv", func(_ *pb.Player, g *model.GameState, w *pb.Warrior, _ map[string]bool) {
			p5SetKV(g, "p5.dispatch:"+w.GetGuid(), "mission")
		}},
		{"expedition-held", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, held map[string]bool) { held[w.GetGuid()] = true }},
		{"pending-zero-training", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) {
			w.TempTraining = &pb.Training{State: proto.Int32(1)}
		}},
		{"pending-training", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) {
			w.TempTraining = &pb.Training{Att: proto.Int32(1)}
		}},
		{"progressed", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) { w.Level = proto.Int32(2) }},
		{"partial-experience", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) { w.Exp = proto.Int64(1) }},
		{"partial-breach", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) {
			w.Breached.Exp = proto.Int32(2)
		}},
		{"breach-time", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) {
			w.Breached.LastAddExpTime = proto.String("1")
		}},
		{"breach-cost", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) {
			w.Breached.CostRecord = proto.String("item_006,2")
		}},
		{"awakening-skill", func(_ *pb.Player, _ *model.GameState, w *pb.Warrior, _ map[string]bool) {
			w.AwakenSkill = proto.String("skill,2")
		}},
		{"not-owned", func(_ *pb.Player, g *model.GameState, _ *pb.Warrior, _ map[string]bool) { g.Warriors = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &pb.Player{HeroWarId: proto.String("main")}
			w := &pb.Warrior{Id: proto.String("hero_003"), Guid: proto.String("material"), Level: proto.Int32(1), Breached: &pb.Breached{Level: proto.Int32(1)}}
			g := &model.GameState{Warriors: []*pb.Warrior{w}}
			held := map[string]bool{}
			if !p5WarriorMaterialAvailable(p, g, w, held) {
				t.Fatal("unprotected original idle donor rejected")
			}
			c.protect(p, g, w, held)
			if p5WarriorMaterialAvailable(p, g, w, held) {
				t.Fatal("protected or pending donor available")
			}
		})
	}
}

// A real first breach spends crystals without increasing the breach level.
// Automatic awakening selection must preserve that card and its investment.
func TestP5WarriorMaterialAwakeSkipsPartialBreach(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeWarriorAwake, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	ids := []string{"z-breach-target", "a-partial-breach", "b-pristine", "c-pristine"}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(70)
		p.Coin = proto.Int64(100000)
		p.WarriorBagSize = proto.Int32(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_003,4;item_006,2;item_322,800;item_315,50;item_312,15")
		if err != nil {
			return err
		}
		for i, card := range gain.GetWars() {
			w, err := p5Warrior(g, card.GetGuid())
			if err != nil {
				return err
			}
			w.Guid = proto.String(ids[i])
			if i == 0 {
				w.Level = proto.Int32(70)
				w.Talent = proto.Int32(3)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	breach := &pb.CSWarriorBreached{Guid: proto.String("a-partial-breach"), Type: proto.Int32(1)}
	if _, err := f.s.warriorBreached(ctx(breach), f.id, breach, 203001); err != nil {
		t.Fatal("real first breach", err)
	}
	p, g := f.profile(t)
	donor, err := p5Warrior(g, breach.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	if donor.GetLevel() != 1 || donor.GetBreached().GetLevel() != 1 || donor.GetBreached().GetExp() != 2 || donor.GetBreached().GetLastAddExpTime() == "" || donor.GetBreached().GetCostRecord() == "" || simpleOwned(p, g, "item_006") != 0 {
		t.Fatal("first breach did not create paid partial progress")
	}
	before := proto.Clone(donor).(*pb.Warrior)
	investment := p5KV(g, "p5.invest:"+donor.GetGuid())
	if investment == "" {
		t.Fatal("real breach investment missing")
	}
	awake := &pb.CSWarriorAwake{Guid: proto.String("z-breach-target")}
	out, err := f.s.warriorAwake(ctx(awake), f.id, awake, 203002)
	if err != nil {
		t.Fatal("two pristine donors must permit awakening", err)
	}
	result := out[0].(*pb.SCWarriorAwake)
	consumed := map[string]bool{}
	for _, cost := range result.GetCost() {
		if cost.GetGuid() != "" {
			consumed[cost.GetGuid()] = true
		}
	}
	if result.GetNewWarrior().GetAwake().GetStage() != 1 || len(consumed) != 2 || !consumed["b-pristine"] || !consumed["c-pristine"] {
		t.Fatal("awakening consumed a paid partially breached donor", consumed)
	}
	_, g = f.profile(t)
	donor, err = p5Warrior(g, breach.GetGuid())
	if err != nil || !proto.Equal(before, donor) || p5KV(g, "p5.invest:"+breach.GetGuid()) != investment {
		t.Fatal("awakening changed protected partial breach or investment", err)
	}
}

// Finite domain prerequisites: Player/target70, target talent3, six real grant
// instances and exact original materials. This proves selection/transactions,
// with no claim of natural acquisition, expedition play or new battle scope.
func TestP5WarriorMaterialAwakeSkipsProtectedAndTalentRejects(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeWarriorAwake, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.P7ExpeditionHero{}); err != nil {
		t.Fatal(err)
	}
	f.s.P5Ready = true
	ids := []string{"z-target", "a-helper", "b-held", "c-free", "d-free", "e-free"}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(70)
		p.Coin = proto.Int64(100000)
		p.WarriorBagSize = proto.Int32(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_003,6;item_005,500;item_322,800;item_315,50;item_312,15")
		if err != nil {
			return err
		}
		for i, card := range gain.GetWars() {
			w, err := p5Warrior(g, card.GetGuid())
			if err != nil {
				return err
			}
			w.Guid = proto.String(ids[i])
			if i == 0 {
				w.Level = proto.Int32(70)
				w.Talent = proto.Int32(3)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	on := &pb.CSHelperOn{Guid: proto.String("a-helper"), Pos: proto.Int32(1)}
	if _, err := f.s.p5HelperOnNative(ctx(on), f.id, on, 202001); err != nil {
		t.Fatal("original helper assignment", err)
	}
	_, g := f.profile(t)
	held, err := p5Warrior(g, "b-held")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(held)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P7ExpeditionHero{PlayerID: f.id, GUID: "b-held", MissionSerial: "finite-domain-hold", Snapshot: raw}).Error; err != nil {
		t.Fatal(err)
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
	awake := &pb.CSWarriorAwake{Guid: proto.String("z-target")}
	out, err := f.s.warriorAwake(ctx(awake), f.id, awake, 202002)
	if err != nil {
		t.Fatal("protected earlier GUID must not hide two available donors", err)
	}
	result := out[0].(*pb.SCWarriorAwake)
	if result.GetNewWarrior().GetAwake().GetStage() != 1 {
		t.Fatal("actual awakening missing")
	}
	consumed := map[string]bool{}
	for _, cost := range result.GetCost() {
		if cost.GetGuid() != "" {
			consumed[cost.GetGuid()] = true
		}
	}
	if len(consumed) != 2 || !consumed["c-free"] || !consumed["d-free"] {
		t.Fatal("original two-donor cost/ordering changed", consumed)
	}
	f.restart(t)
	saved := stable()
	replay, err := f.s.warriorAwake(ctx(awake), f.id, awake, 202002)
	if err != nil || !proto.Equal(replay[0], out[0]) || saved != stable() {
		t.Fatal("current awake cold replay", err)
	}
	talent := func(donor string) *pb.CSWarriorTalent {
		return &pb.CSWarriorTalent{Guid: proto.String("z-target"), Talent: proto.String("4"), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(40000)}, {Id: proto.String("item_005"), Num: proto.Int32(500)}, {Id: proto.String("hero_003"), Guid: proto.String(donor), Num: proto.Int32(1)}}}
	}
	for _, donor := range []string{"a-helper", "b-held"} {
		b := talent(donor)
		if _, err := f.s.p5WarriorTalent(ctx(b), f.id, b, 202003); err == nil {
			t.Fatal("explicit protected donor accepted", donor)
		}
		if saved != stable() {
			t.Fatal("rejected whole request spent or mutated", donor)
		}
	}
	valid := talent("e-free")
	done, err := f.s.p5WarriorTalent(ctx(valid), f.id, valid, 202003)
	if err != nil || done[0].(*pb.SCWarriorTalent).GetGuid().GetTalent() != 4 {
		t.Fatal("remaining unprotected donor unusable", err)
	}
	f.restart(t)
	p, g := f.profile(t)
	if p.GetCoin() != 60000 || simpleOwned(p, g, "item_005") != 0 || simpleOwned(p, g, "item_322") != 0 || simpleOwned(p, g, "item_315") != 0 || simpleOwned(p, g, "item_312") != 0 {
		t.Fatal("original talent/awake costs changed")
	}
	for _, guid := range []string{"z-target", "a-helper", "b-held"} {
		if _, err := p5Warrior(g, guid); err != nil {
			t.Fatal("protected/current card missing", guid, err)
		}
	}
	if p5KV(g, "p5.helper:1") != "a-helper" {
		t.Fatal("helper assignment changed")
	}
	var holds int64
	if err := db.GetIns().Model(&db.P7ExpeditionHero{}).Where("player_id=? AND guid=?", f.id, "b-held").Count(&holds).Error; err != nil || holds != 1 {
		t.Fatal("expedition ownership released by growth", err)
	}
}

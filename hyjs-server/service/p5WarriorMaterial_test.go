package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP5WarriorMaterialOriginalEXPTrackedResetRetryAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	// Finite domain fixture only. The original item067 choice index1 supplies
	// these same hero011 units; the HTTP check can exercise that source directly.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(16)
		_, err := f.s.GrantFixedReward(p, g, "coin,10000;diamond,100;hero_011,4")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	_, initialG := f.profile(t)
	var units []*pb.Warrior
	for _, unit := range initialG.Warriors {
		if unit.GetId() == "hero_011" {
			units = append(units, unit)
		}
	}
	if len(units) != 4 {
		t.Fatal("finite original warrior fixture")
	}
	donorGUID, spentGUID, targetGUID := units[0].GetGuid(), units[1].GetGuid(), units[2].GetGuid()
	feed := &pb.CSWarriorStrengthen{Guid: proto.String(donorGUID), Cost: []*pb.BaseResource{{Id: units[1].Id, Guid: units[1].Guid}}}
	first, err := f.s.p5WarriorStrengthen(ctx(feed), f.id, feed, 772001)
	if err != nil {
		t.Fatal("first real donor growth", err)
	}
	donor := first[0].(*pb.SCWarriorStrengthen).GetGuid()
	if donor.GetLevel() != 4 || donor.GetExp() != 151 {
		t.Fatal("original quality20 1000-EXP strict threshold", donor)
	}
	p, donorG := f.profile(t)
	if p.GetCoin() != 9000 {
		t.Fatal("first original coin fee", p.GetCoin())
	}
	if _, err = p5InvestmentState(donorG, donorGUID, donor); err != nil {
		t.Fatal("actual donor investment missing", err)
	}
	// Validate rollback after the valid donor has already been examined. No
	// audit, cost inheritance, money or removal may escape a failed mixed batch.
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.lock:"+units[3].GetGuid(), "1")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := f.profile(t)
	beforeRaw, _ := json.Marshal(beforeG)
	mixed := &pb.CSWarriorStrengthen{Guid: proto.String(targetGUID), Cost: []*pb.BaseResource{{Id: units[0].Id, Guid: units[0].Guid}, {Id: units[3].Id, Guid: units[3].Guid}}}
	if _, err = f.s.p5WarriorStrengthen(ctx(mixed), f.id, mixed, 772002); err == nil {
		t.Fatal("locked late material accepted")
	}
	afterP, afterG := f.profile(t)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("mixed batch partially committed")
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.lock:"+units[3].GetGuid(), "")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	strengthen := &pb.CSWarriorStrengthen{Guid: proto.String(targetGUID), Cost: []*pb.BaseResource{{Id: units[0].Id, Guid: units[0].Guid}}}
	var wg sync.WaitGroup
	replies := make(chan proto.Message, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.p5WarriorStrengthen(ctx(strengthen), f.id, strengthen, 772002)
			if e != nil {
				failures <- e
				return
			}
			replies <- out[0]
		}()
	}
	wg.Wait()
	close(replies)
	close(failures)
	for e := range failures {
		t.Error(e)
	}
	var original proto.Message
	for out := range replies {
		if original == nil {
			original = out
		} else if !proto.Equal(original, out) {
			t.Error("same-stamp result changed")
		}
	}
	if original == nil {
		t.Fatal("grown donor never accepted")
	}
	p, grownG := f.profile(t)
	target, err := p5Warrior(grownG, targetGUID)
	if err != nil || target.GetLevel() != 5 || target.GetExp() != 271 || p.GetCoin() != 7000 {
		t.Fatal("original base1000 plus stored1000 EXP and fee", target, p.GetCoin(), err)
	}
	ledger, err := p5InvestmentState(grownG, targetGUID, target)
	if err != nil {
		t.Fatal(err)
	}
	amounts := map[string]int32{}
	for _, cost := range ledger.Costs {
		amounts[cost.GetId()] += cost.GetNum()
	}
	if len(amounts) != 2 || amounts["coin"] != 3000 || amounts["hero_011"] != 2 {
		t.Fatal("nested costs not flattened exactly", amounts)
	}
	var audit p5WarriorMaterialAudit
	if p5KV(grownG, "p5.invest:"+donorGUID) != "" || json.Unmarshal([]byte(p5KV(grownG, constUtil.P5WarriorMaterialAuditPrefix+donorGUID)), &audit) != nil || audit.TargetGUID != targetGUID || audit.Policy != constUtil.P5WarriorMaterialPolicy {
		t.Fatal("consumed active ledger or transfer audit incorrect")
	}
	if _, err = f.s.p5WarriorStrengthen(ctx(feed), f.id, feed, 772001); err == nil {
		t.Fatal("old donor response resurrected consumed GUID")
	}
	f.restart(t)
	cold, err := f.s.p5WarriorStrengthen(ctx(strengthen), f.id, strengthen, 772002)
	if err != nil || !proto.Equal(cold[0], original) {
		t.Fatal("cold committed growth response", err)
	}
	reset := &pb.CSWarriorReset{Guid: proto.String(targetGUID)}
	if _, err = f.s.p5WarriorReset(ctx(reset), f.id, reset, 772003); err != nil {
		t.Fatal("real target reset", err)
	}
	p, resetG := f.profile(t)
	if p.GetCoin() != 10000 || p.GetDiamond() != 50 {
		t.Fatal("real3000 fee refund and original50 diamond reset", p.GetCoin(), p.GetDiamond())
	}
	count := 0
	for _, unit := range resetG.Warriors {
		if unit.GetGuid() == donorGUID || unit.GetGuid() == spentGUID || unit.GetGuid() == targetGUID {
			t.Fatal("reset restored a consumed GUID")
		}
		if unit.GetId() == "hero_011" {
			count++
			if unit.GetLevel() != 1 || unit.GetExp() != 0 {
				t.Fatal("refunded units not pristine")
			}
		}
	}
	if count != 1 || simpleOwned(p, resetG, "frag_hero_011") != 80 {
		t.Fatal("upgrade donors become EXP; target becomes80 fragments; unused card survives", count, simpleOwned(p, resetG, "frag_hero_011"))
	}
	resetRaw, _ := json.Marshal(resetG)
	if _, err = f.s.p5WarriorReset(ctx(reset), f.id, reset, 772003); err != nil {
		t.Fatal("reset receipt retry", err)
	}
	finalP, finalG := f.profile(t)
	finalRaw, _ := json.Marshal(finalG)
	if !proto.Equal(p, finalP) || string(resetRaw) != string(finalRaw) {
		t.Fatal("reset retry returned costs twice")
	}
	if _, err = f.s.p5WarriorStrengthen(ctx(strengthen), f.id, strengthen, 772002); err == nil {
		t.Fatal("old pre-reset target projection accepted")
	}
}

func TestP5WarriorMaterialMissingPartialPendingAndOwnershipProtected(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	baseline := &pb.Warrior{Guid: proto.String("donor"), Id: proto.String("hero_011"), Level: proto.Int32(1), Exp: proto.Int64(0)}
	current := proto.Clone(baseline).(*pb.Warrior)
	current.Level, current.Exp = proto.Int32(4), proto.Int64(151)
	request := &pb.CSWarriorStrengthen{Guid: proto.String("target"), Cost: []*pb.BaseResource{{Id: proto.String("hero_011"), Guid: proto.String("donor")}}}
	for _, mode := range []string{"missing", "exp-only-missing", "fingerprint", "partial-origin", "baseline-breach-exp", "baseline-breach-time", "baseline-breach-cost", "baseline-awaken-skill", "pending-training", "locked", "helper", "dispatch", "main", "foreign-request", "wrong-consumer", "already-transferred", "unsupported-cost", "timed-food-cost"} {
		t.Run(mode, func(t *testing.T) {
			p := &pb.Player{}
			material := proto.Clone(current).(*pb.Warrior)
			before := &model.GameState{Warriors: []*pb.Warrior{material}}
			if err := RecordP5Investment(before, "donor", baseline, material, []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1000)}}); err != nil {
				t.Fatal(err)
			}
			var req proto.Message = request
			switch mode {
			case "missing", "exp-only-missing":
				p5SetKV(before, "p5.invest:donor", "")
				if mode == "exp-only-missing" {
					material.Level = proto.Int32(1)
				}
			case "fingerprint":
				material.Exp = proto.Int64(152)
			case "partial-origin", "baseline-breach-exp", "baseline-breach-time", "baseline-breach-cost", "baseline-awaken-skill", "unsupported-cost", "timed-food-cost":
				state, err := p5InvestmentState(before, "donor", material)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "unsupported-cost" {
					state.Costs[0].Id = proto.String("unknown-resource")
				} else if mode == "timed-food-cost" {
					state.Costs[0].Id = proto.String("FishItem_035")
				} else {
					grown := proto.Clone(baseline).(*pb.Warrior)
					switch mode {
					case "partial-origin":
						grown.Level = proto.Int32(2)
					case "baseline-breach-exp":
						grown.Breached = &pb.Breached{Level: proto.Int32(1), Exp: proto.Int32(1)}
					case "baseline-breach-time":
						grown.Breached = &pb.Breached{Level: proto.Int32(1), LastAddExpTime: proto.String("123")}
					case "baseline-breach-cost":
						grown.Breached = &pb.Breached{Level: proto.Int32(1), CostRecord: proto.String("item_006,1")}
					case "baseline-awaken-skill":
						grown.AwakenSkill = proto.String("0003_1,2")
					}
					state.Baseline, _ = json.Marshal(grown)
				}
				raw, _ := json.Marshal(state)
				p5SetKV(before, "p5.invest:donor", string(raw))
			case "pending-training":
				material.TempTraining = &pb.Training{State: proto.Int32(1), Att: proto.Int32(1)}
				// Keep a matching ledger so the unfinished-choice guard itself is exercised.
				p5SetKV(before, "p5.invest:donor", "")
				if err := RecordP5Investment(before, "donor", baseline, material, []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1000)}}); err != nil {
					t.Fatal(err)
				}
			case "locked":
				p5SetKV(before, "p5.lock:donor", "1")
			case "helper":
				p5SetKV(before, "p5.helper:1", "donor")
			case "dispatch":
				p5SetKV(before, "p5.dispatch:donor", "active")
			case "main":
				p.HeroWarId = proto.String("donor")
			case "foreign-request":
				req = &pb.CSWarriorStrengthen{Guid: proto.String("target"), Cost: []*pb.BaseResource{{Id: proto.String("hero_011"), Guid: proto.String("other")}}}
			case "wrong-consumer":
				req = &pb.CSWarriorTalent{Guid: proto.String("target")}
			case "already-transferred":
				p5SetKV(before, constUtil.P5WarriorMaterialAuditPrefix+"donor", "old-audit")
			}
			encoded, _ := json.Marshal(before)
			after := &model.GameState{}
			if err := json.Unmarshal(encoded, after); err != nil {
				t.Fatal(err)
			}
			after.Warriors = nil
			unchanged, _ := json.Marshal(after)
			if _, err := s.p5WarriorMaterialCosts(p, before, after, material, "target", req); err == nil {
				t.Fatal("invalid/advanced/untracked material accepted")
			}
			actual, _ := json.Marshal(after)
			if string(unchanged) != string(actual) {
				t.Fatal("rejection changed transfer audit or ledger")
			}
		})
	}
}

// The ledger transfer depends on complete ownership/fingerprint/cost evidence,
// not a separate allow flag for every passive growth field. These are typed
// ledger vectors; actual producers retain their independently tested rules.
func TestP5WarriorMaterialAllTrackedGrowthClassesTransferCosts(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	baseline := &pb.Warrior{Id: proto.String("hero_003"), Guid: proto.String("donor"), Level: proto.Int32(1), Exp: proto.Int64(0), Breached: &pb.Breached{Level: proto.Int32(1)}, Training: &pb.Training{}, Awake: &pb.Awake{}}
	for _, kind := range []string{"level-exp", "saved-training", "breach-partial-exp", "talent", "awake", "fusion", "permanent-cooking", "specialty", "combined"} {
		t.Run(kind, func(t *testing.T) {
			current := proto.Clone(baseline).(*pb.Warrior)
			current.Level = proto.Int32(75)
			if kind == "level-exp" {
				current.Level, current.Exp = proto.Int32(4), proto.Int64(151)
			}
			if kind == "saved-training" || kind == "combined" {
				current.Training = &pb.Training{State: proto.Int32(0), Att: proto.Int32(1)}
			}
			if kind == "breach-partial-exp" || kind == "combined" {
				current.Breached = &pb.Breached{Level: proto.Int32(1), Exp: proto.Int32(1), LastAddExpTime: proto.String("123"), CostRecord: proto.String("item_006,1")}
			}
			if kind == "talent" || kind == "combined" {
				current.Talent = proto.Int32(1)
			}
			if kind == "awake" || kind == "combined" {
				current.Awake = &pb.Awake{Stage: proto.Int32(1)}
			}
			if kind == "fusion" {
				current.Awake = &pb.Awake{Stage: proto.Int32(2), FuseLevel: proto.Int32(1), FuseExp: proto.Int32(1)}
				current.AwakenSkill = proto.String("0003_1,2")
			}
			if kind == "permanent-cooking" || kind == "combined" {
				current.Cooking = &pb.Cooking{Prop: &pb.BaseProp{Pdef: proto.Float64(1)}}
			}
			if kind == "specialty" || kind == "combined" {
				current.SkillRefine = []*pb.SkillRefine{{RoleGuid: current.Guid, Type: proto.Int32(1), Level: proto.Int32(1), StrengthLevel: proto.Int32(2)}, {RoleGuid: current.Guid, Type: proto.Int32(2), Level: proto.Int32(1), StrengthLevel: proto.Int32(1)}, {RoleGuid: current.Guid, Type: proto.Int32(3), Level: proto.Int32(1), StrengthLevel: proto.Int32(1)}}
			}
			before := &model.GameState{Warriors: []*pb.Warrior{current}}
			costs := []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1000)}, {Id: proto.String("FishItem_027"), Num: proto.Int32(3)}}
			if err := RecordP5Investment(before, "donor", baseline, current, costs); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(before)
			after := &model.GameState{}
			if err := json.Unmarshal(encoded, after); err != nil {
				t.Fatal(err)
			}
			after.Warriors = nil
			request := &pb.CSWarriorStrengthen{Guid: proto.String("target"), Cost: []*pb.BaseResource{{Id: current.Id, Guid: current.Guid}}}
			got, err := s.p5WarriorMaterialCosts(&pb.Player{}, before, after, current, "target", request)
			if err != nil || len(got) != 2 {
				t.Fatal("complete passive-growth ledger rejected", err)
			}
			amounts := map[string]int32{}
			for _, cost := range got {
				amounts[cost.GetId()] = cost.GetNum()
			}
			if amounts["coin"] != 1000 || amounts["FishItem_027"] != 3 || p5KV(after, "p5.invest:donor") != "" {
				t.Fatal("transfer changed actual costs or retained active donor ledger", amounts)
			}
			if len(after.Cooking) != 0 {
				t.Fatal("transfer refunded cooking immediately")
			}
		})
	}
}

func TestP5WarriorMaterialRealTrainingCookingOddMergeResetOnceAndCold(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitCooking} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(25)
		_, err := f.s.GrantFixedReward(p, g, "coin,100000;diamond,100;hero_011,3;hero_exp_004,1;item_007,3;FishItem_027,3")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	_, initial := f.profile(t)
	var units []*pb.Warrior
	var expCard *pb.Warrior
	for _, w := range initial.Warriors {
		if w.GetId() == "hero_011" {
			units = append(units, w)
		} else if w.GetId() == "hero_exp_004" {
			expCard = w
		}
	}
	if len(units) != 3 || expCard == nil {
		t.Fatal("finite original material fixture")
	}
	donor, other, target := units[0], units[1], units[2]
	feed := &pb.CSWarriorStrengthen{Guid: donor.Guid, Cost: []*pb.BaseResource{{Id: expCard.Id, Guid: expCard.Guid}}}
	if _, err := f.s.p5WarriorStrengthen(ctx(feed), f.id, feed, 773001); err != nil {
		t.Fatal("real original20000 EXP feed", err)
	}
	train := &pb.CSWarriorTraining{Guid: donor.Guid, CostType: proto.Int32(3), NumType: proto.Int32(1)}
	if _, err := f.s.warriorTraining(ctx(train), f.id, train, 773002); err != nil {
		t.Fatal("real advanced training", err)
	}
	consume := &pb.CSWarriorStrengthen{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: donor.Id, Guid: donor.Guid}, {Id: other.Id, Guid: other.Guid}}}
	pendingP, pendingG := f.profile(t)
	pendingRaw, _ := json.Marshal(pendingG)
	if _, err := f.s.p5WarriorStrengthen(ctx(consume), f.id, consume, 773003); err == nil {
		t.Fatal("unconfirmed training material accepted")
	}
	unchangedP, unchangedG := f.profile(t)
	unchangedRaw, _ := json.Marshal(unchangedG)
	if !proto.Equal(pendingP, unchangedP) || string(pendingRaw) != string(unchangedRaw) {
		t.Fatal("pending training rejection partially consumed")
	}
	confirm := &pb.CSWarriorTrainingReplace{Guid: donor.Guid, Result: proto.Int32(1)}
	if _, err := f.s.warriorTrainingReplace(ctx(confirm), f.id, confirm, 773004); err != nil {
		t.Fatal("real training confirmation", err)
	}
	_, trainedG := f.profile(t)
	trained, err := p5Warrior(trainedG, donor.GetGuid())
	if err != nil || growthValues(trained.GetTraining()) == [4]int32{} {
		t.Fatal("advanced training did not produce saved growth", err)
	}
	// Each of the two donors and the target really eats one dish. Three odd
	// paid ledgers merge to one final floor(3/2)=1 refund, with no early grant.
	var lastFood *pb.CSFishUsing
	for i, w := range []*pb.Warrior{target, donor, other} {
		lastFood = &pb.CSFishUsing{HeroGuid: w.Guid, CostItem: &pb.BaseResource{Id: proto.String("FishItem_027"), Num: proto.Int32(1)}}
		if _, err = f.s.fishUsing(ctx(lastFood), f.id, lastFood, int64(773005+i)); err != nil {
			t.Fatal("actual permanent dish use", err)
		}
	}
	out, err := f.s.p5WarriorStrengthen(ctx(consume), f.id, consume, 773008)
	if err != nil {
		t.Fatal("confirmed training/food donors rejected", err)
	}
	result := out[0].(*pb.SCWarriorStrengthen)
	if result.GetGuid().GetLevel() != 12 || result.GetGuid().GetExp() != 4088 || result.GetGuid().GetCooking().GetProp().GetPdef() != 30 || growthValues(result.GetGuid().GetTraining()) != [4]int32{} {
		t.Fatal("consumer changed originalEXP-only inheritance", result.GetGuid())
	}
	p, grownG := f.profile(t)
	ledger, err := p5InvestmentState(grownG, target.GetGuid(), result.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	amounts := map[string]int32{}
	for _, c := range ledger.Costs {
		amounts[c.GetId()] = c.GetNum()
	}
	if len(amounts) != 6 || amounts["coin"] != 42000 || amounts["diamond"] != 5 || amounts["hero_011"] != 2 || amounts["hero_exp_004"] != 1 || amounts["item_007"] != 3 || amounts["FishItem_027"] != 3 || p.GetCoin() != 58000 || p.GetDiamond() != 95 || len(grownG.Cooking) != 0 {
		t.Fatal("multi-domain actual costs not preserved or early refund", amounts, p)
	}
	if _, err = f.s.warriorTrainingReplace(ctx(confirm), f.id, confirm, 773004); err == nil {
		t.Fatal("old training callback resurrected donor")
	}
	if _, err = f.s.fishUsing(ctx(lastFood), f.id, lastFood, 773007); err == nil {
		t.Fatal("old cooking callback resurrected donor")
	}
	f.restart(t)
	if cold, e := f.s.p5WarriorStrengthen(ctx(consume), f.id, consume, 773008); e != nil || !proto.Equal(cold[0], result) {
		t.Fatal("cold multi-domain receipt", e)
	}
	reset := &pb.CSWarriorReset{Guid: target.Guid}
	resetOut, err := f.s.p5WarriorReset(ctx(reset), f.id, reset, 773009)
	if err != nil {
		t.Fatal("multi-domain actual investment reset", err)
	}
	p, resetG := f.profile(t)
	var dishes, drills int32
	for _, r := range resetG.Cooking {
		if r.GetId() == "FishItem_027" {
			dishes += r.GetNum()
		}
	}
	for _, r := range resetG.Items {
		if r.GetId() == "item_007" {
			drills += r.GetNum()
		}
	}
	if dishes != 1 || drills != 3 || p.GetCoin() != 100000 || p.GetDiamond() != 45 {
		t.Fatal("single final aggregate rounddown/refund; training diamonds remain spent", dishes, drills, p)
	}
	refundHero, refundEXP := 0, 0
	for _, w := range resetG.Warriors {
		if w.GetGuid() == donor.GetGuid() || w.GetGuid() == other.GetGuid() || w.GetGuid() == expCard.GetGuid() {
			t.Fatal("refund restored consumed GUID")
		}
		if w.GetId() == "hero_011" && w.GetGuid() != target.GetGuid() {
			refundHero++
		}
		if w.GetId() == "hero_exp_002" {
			refundEXP++
		}
	}
	if refundHero != 0 || refundEXP != 4 || simpleOwned(p, resetG, "frag_hero_011") != 80 {
		t.Fatal("target80 fragments and20000 paidEXP normalized to four5000 crystals", refundHero, refundEXP, simpleOwned(p, resetG, "frag_hero_011"))
	}
	savedRaw, _ := json.Marshal(resetG)
	if again, e := f.s.p5WarriorReset(ctx(reset), f.id, reset, 773009); e != nil || !proto.Equal(again[0], resetOut[0]) {
		t.Fatal("reset receipt replay", e)
	}
	finalP, finalG := f.profile(t)
	finalRaw, _ := json.Marshal(finalG)
	if !proto.Equal(p, finalP) || string(savedRaw) != string(finalRaw) {
		t.Fatal("repeat reset refunded coin/dishes twice")
	}
}

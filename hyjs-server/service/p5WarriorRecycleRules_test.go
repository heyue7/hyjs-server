package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP5WarriorOriginalRefundPolicy(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	w := &pb.Warrior{Id: proto.String("hero_011"), Guid: proto.String("refund-policy")}
	g := &model.GameState{}
	state := &p5Investment{RefundVersion: 1}
	resource := func(key string, n int32) *pb.BaseResource {
		return &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(n)}
	}
	training := []*pb.BaseResource{resource("coin", 1000), resource("diamond", 3), resource("item_013", 2)}
	talent := []*pb.BaseResource{resource("coin", 2000), resource("hero_011", 1), resource("item_005", 10)}
	upgrade := []*pb.BaseResource{resource("coin", 109000), resource("hero_exp_004", 5)}
	for _, op := range []struct {
		name  string
		costs []*pb.BaseResource
	}{{"CSWarriorTraining", training}, {"CSWarriorTalent", talent}, {"CSWarriorStrengthen", upgrade}} {
		if err := p5ClassifyWarriorInvestment(state, w, w, op.costs, []string{op.name}); err != nil {
			t.Fatal(err)
		}
		state.Costs = append(state.Costs, op.costs...)
	}
	state.Costs = append(state.Costs, resource("item_006", 7))
	if err := p5TrackBreachRefund(g, w.GetGuid(), []*pb.BaseResource{resource("item_006", 4)}, true, false); err != nil {
		t.Fatal(err)
	}
	if err := p5TrackBreachRefund(g, w.GetGuid(), []*pb.BaseResource{resource("item_006", 2)}, false, false); err != nil {
		t.Fatal(err)
	}
	if err := p5TrackBreachRefund(g, w.GetGuid(), nil, false, true); err != nil {
		t.Fatal(err)
	}
	if err := p5TrackBreachRefund(g, w.GetGuid(), []*pb.BaseResource{resource("item_006", 1)}, false, false); err != nil {
		t.Fatal(err)
	}
	refund, err := s.p5WarriorRefundCosts(g, state, w)
	if err != nil {
		t.Fatal(err)
	}
	amounts := map[string]int32{}
	for _, cost := range refund {
		amounts[cost.GetId()] += cost.GetNum()
	}
	if amounts["coin"] != 109000 || amounts["diamond"] != 0 || amounts["item_013"] != 2 || amounts["item_005"] != 10 || amounts["item_006"] != 4 || amounts["frag_hero_011"] != 80 || amounts["item_066"] != 1 || amounts["hero_exp_002"] != 1 || amounts["hero_011"] != 0 || amounts["hero_exp_004"] != 0 {
		t.Fatal("original purpose/fragment/EXP/breach refund mismatch", amounts)
	}
	state.RefundVersion = 0
	if _, err := s.p5WarriorRefundCosts(g, state, w); err == nil {
		t.Fatal("unclassified legacy costs were guessed")
	}
	gearBefore := &pb.Equipment{EquipId: proto.String("equip_037"), Guid: proto.String("forge-policy")}
	gearAfter := proto.Clone(gearBefore).(*pb.Equipment)
	gearAfter.ForgeLevel = proto.Int32(1)
	gearState := &p5Investment{RefundVersion: 1}
	paidSuccess := []*pb.BaseResource{resource("coin", 1000), resource("frag_equip_037", 6)}
	paidFailure := []*pb.BaseResource{resource("coin", 2000), resource("frag_equip_037", 8)}
	if err := p5ClassifyWarriorInvestment(gearState, gearBefore, gearAfter, paidSuccess, []string{"CSEquipForge"}); err != nil {
		t.Fatal(err)
	}
	if err := p5ClassifyWarriorInvestment(gearState, gearAfter, gearAfter, paidFailure, []string{"CSEquipForge"}); err != nil {
		t.Fatal(err)
	}
	gearState.Costs = []*pb.BaseResource{resource("coin", 3500), resource("frag_equip_037", 14)}
	gearRefund, err := p5EquipmentRefundCosts(gearState, gearAfter)
	if err != nil {
		t.Fatal(err)
	}
	gearAmounts := map[string]int32{}
	for _, cost := range gearRefund {
		gearAmounts[cost.GetId()] += cost.GetNum()
	}
	if gearAmounts["coin"] != 500 || gearAmounts["frag_equip_037"] != 6 {
		t.Fatal("failed forge or forge coin refunded", gearAmounts)
	}
}

// TestP5WarriorOriginalResetTransaction uses two real strength requests followed
// by reset and cold retry. Finite input resources avoid replaying a new-user route.
func TestP5WarriorOriginalResetTransaction(t *testing.T) {
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
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(100000)
		p.Diamond = proto.Int64(100)
		_, err := f.s.GrantFixedReward(p, g, "hero_011,2;hero_exp_003,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	requestContext := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	_, g := f.profile(t)
	var target, donor, experience *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_011" {
			if donor == nil {
				donor = w
			} else {
				target = w
			}
		}
		if w.GetId() == "hero_exp_003" {
			experience = w
		}
	}
	if target == nil || donor == nil || experience == nil {
		t.Fatal("finite owned fixture missing")
	}
	feed := &pb.CSWarriorStrengthen{Guid: donor.Guid, Cost: []*pb.BaseResource{{Id: experience.Id, Guid: experience.Guid, Num: proto.Int32(1)}}}
	if _, err := f.s.p5WarriorStrengthen(requestContext(feed), f.id, feed, 987001); err != nil {
		t.Fatal(err)
	}
	feed = &pb.CSWarriorStrengthen{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: donor.Id, Guid: donor.Guid, Num: proto.Int32(1)}}}
	if _, err := f.s.p5WarriorStrengthen(requestContext(feed), f.id, feed, 987002); err != nil {
		t.Fatal(err)
	}
	reset := &pb.CSWarriorReset{Guid: target.Guid}
	out, err := f.s.p5WarriorReset(requestContext(reset), f.id, reset, 987003)
	if err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	if p.GetCoin() != 100000 || p.GetDiamond() != 50 || simpleOwned(p, g, "frag_hero_011") != 80 {
		t.Fatal("original coin and base fragments", p, g.Items)
	}
	for _, w := range g.Warriors {
		if w.GetGuid() == target.GetGuid() || w.GetGuid() == donor.GetGuid() || w.GetId() == "hero_011" {
			t.Fatal("reset retained original or donor whole card")
		}
	}
	result := out[0].(*pb.SCWarriorReset)
	if len(result.GetWarriors()) != 0 || len(result.GetCost()) != 2 {
		t.Fatal("client removal contract", result)
	}
	if len(result.GetGet().GetWars()) != 2 {
		t.Fatal("nested donor EXP was not converted once", result.GetGet())
	}
	before, _ := json.Marshal(g)
	f.restart(t)
	again, err := f.s.p5WarriorReset(requestContext(reset), f.id, reset, 987003)
	if err != nil || !proto.Equal(again[0], out[0]) {
		t.Fatal("cold receipt", err)
	}
	afterP, afterG := f.profile(t)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(p, afterP) || string(before) != string(after) {
		t.Fatal("cold retry refunded twice")
	}
	blocked := &pb.CSWarriorDecompose{Guid: []string{p.GetHeroWarId()}}
	if _, err := f.s.p5WarriorRecycle(requestContext(blocked), f.id, blocked.GetGuid(), true, 987004, blocked); err == nil {
		t.Fatal("protected main warrior was decomposed")
	}
}

func TestP5OriginalMainEquipmentResetTransaction(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitGems, f.s.InitEquipmentMutations} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(100), proto.Int64(100000), proto.Int64(200)
		main, err := p5Warrior(g, p.GetHeroWarId())
		if err != nil {
			return err
		}
		main.Level, main.Exp = proto.Int32(60), proto.Int64(123)
		g.Gems = []*pb.SimpleItem{{Id: proto.String("MineItem_019"), Num: proto.Int32(1)}}
		_, err = f.s.GrantFixedReward(p, g, "item_007,10;equip_037,1;hero_011,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	requestContext := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	p, _ := f.profile(t)
	train := &pb.CSWarriorTraining{Guid: p.HeroWarId, CostType: proto.Int32(3), NumType: proto.Int32(1)}
	if _, err := f.s.warriorTraining(requestContext(train), f.id, train, 988001); err != nil {
		t.Fatal("main training", err)
	}
	if _, err := f.s.SwapP5MainHero(f.ctx, f.id, 101, 988002); err != nil {
		t.Fatal("actual form swap", err)
	}
	p, g := f.profile(t)
	current, _ := p5Warrior(g, p.GetHeroWarId())
	shape := current.GetId()
	mainReset := &pb.CSWarriorReset{Guid: p.HeroWarId}
	mainOut, err := f.s.p5WarriorReset(requestContext(mainReset), f.id, mainReset, 988003)
	if err != nil {
		t.Fatal("main reset", err)
	}
	p, g = f.profile(t)
	current, err = p5Warrior(g, p.GetHeroWarId())
	if err != nil || current.GetId() != shape || current.GetLevel() != 60 || current.GetExp() != 123 || p.GetDiamond() != 145 || simpleOwned(p, g, "item_007") != 10 || trainingPending(current.GetTempTraining()) || growthValues(current.GetTraining()) != [4]int32{} {
		t.Fatal("main identity, level, shape or original refund mismatch", current, p, err)
	}
	if len(mainOut[0].(*pb.SCWarriorReset).GetWarriors()) != 1 || len(mainOut[0].(*pb.SCWarriorReset).GetCost()) != 1 {
		t.Fatal("main client update/remove contract")
	}
	var equip *pb.Equipment
	var pristine *pb.Warrior
	for _, owned := range g.Equipment {
		if owned.GetEquipId() == "equip_037" {
			equip = owned
		}
	}
	for _, owned := range g.Warriors {
		if owned.GetId() == "hero_011" {
			pristine = owned
		}
	}
	if equip == nil || pristine == nil {
		t.Fatal("finite target fixture missing")
	}
	mount := &pb.CSGemMount{Guid: equip.Guid, GemId: proto.String("MineItem_019"), Pos: proto.Int32(0)}
	if _, err := f.s.gemMount(requestContext(mount), f.id, mount, 988004); err != nil {
		t.Fatal("actual gem mount", err)
	}
	strengthen := &pb.CSEquipStrengthenOneKey{Guid: equip.Guid}
	if _, err := f.s.p5StrengthenOne(requestContext(strengthen), f.id, strengthen, 988005); err != nil {
		t.Fatal("actual equipment strength", err)
	}
	gearReset := &pb.CSEquipReBuild{Guid: equip.Guid}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Gems = append(g.Gems, &pb.SimpleItem{Id: proto.String("MineItem_019"), Num: proto.Int32(math.MaxInt32)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	blockedP, blockedG := f.profile(t)
	blockedRaw, _ := json.Marshal(blockedG)
	if _, err := f.s.p5EquipmentReset(requestContext(gearReset), f.id, gearReset, 988006); err == nil {
		t.Fatal("gem capacity overflow accepted")
	}
	stillP, stillG := f.profile(t)
	stillRaw, _ := json.Marshal(stillG)
	if !proto.Equal(blockedP, stillP) || string(blockedRaw) != string(stillRaw) {
		t.Fatal("failed gem return committed fee, refund or removal")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		filtered := g.Gems[:0]
		for _, gem := range g.Gems {
			if gem.GetId() != "MineItem_019" {
				filtered = append(filtered, gem)
			}
		}
		g.Gems = filtered
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	gearOut, err := f.s.p5EquipmentReset(requestContext(gearReset), f.id, gearReset, 988006)
	if err != nil {
		t.Fatal("equipment reset", err)
	}
	p, g = f.profile(t)
	if p.GetCoin() != 100000 || p.GetDiamond() != 95 || simpleOwned(p, g, "frag_equip_037") != 60 {
		t.Fatal("equipment fragments and paid investment", p)
	}
	for _, owned := range g.Equipment {
		if owned.GetGuid() == equip.GetGuid() {
			t.Fatal("reset retained original equipment")
		}
	}
	gemCount := int32(0)
	for _, gem := range g.Gems {
		if gem.GetId() == "MineItem_019" {
			gemCount += gem.GetNum()
		}
	}
	if gemCount != 1 || len(gearOut[0].(*pb.SCEquipReBuild).GetCost()) != 2 || len(gearOut[0].(*pb.SCEquipReBuild).GetGet().GetEquips()) != 0 {
		t.Fatal("physical gem or client removal mismatch")
	}
	// A completely untrained owned card has no legacy classification requirement.
	pristineReset := &pb.CSWarriorReset{Guid: pristine.Guid}
	if _, err := f.s.p5WarriorReset(requestContext(pristineReset), f.id, pristineReset, 988007); err != nil {
		t.Fatal("pristine no-ledger reset", err)
	}
	p, g = f.profile(t)
	if p.GetDiamond() != 45 || simpleOwned(p, g, "frag_hero_011") != 80 {
		t.Fatal("pristine fragment return")
	}
	before, _ := json.Marshal(g)
	f.restart(t)
	if again, err := f.s.p5EquipmentReset(requestContext(gearReset), f.id, gearReset, 988006); err != nil || !proto.Equal(again[0], gearOut[0]) {
		t.Fatal("equipment cold once", err)
	}
	if again, err := f.s.p5WarriorReset(requestContext(mainReset), f.id, mainReset, 988003); err != nil || !proto.Equal(again[0], mainOut[0]) {
		t.Fatal("main cold once", err)
	}
	afterP, afterG := f.profile(t)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(p, afterP) || string(before) != string(after) {
		t.Fatal("cold main/equipment refund repeated")
	}
}

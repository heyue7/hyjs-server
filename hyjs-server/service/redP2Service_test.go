package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func newP2Test(t *testing.T) (*GameService, context.Context) {
	t.Helper()
	s, ctx, _ := newDailyCopyTest(t)
	initializeP2Test(t, s, ctx)
	return s, ctx
}
func initializeP2Test(t *testing.T, s *GameService, ctx context.Context) {
	t.Helper()
	for _, initialize := range []func(context.Context) error{s.InitializeGameplayPools, s.InitializeP2, s.InitializeTurntable} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
func p2Profile(t *testing.T, id int64) (*pb.Player, *model.GameState) {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	p, g, e := decodeProfile(&row)
	if e != nil {
		t.Fatal(e)
	}
	return p, g
}
func p2Funding(t *testing.T, s *GameService, ctx context.Context, id int64, content string) {
	t.Helper()
	if e := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { _, e := s.GrantFixedReward(p, g, content); return e }); e != nil {
		t.Fatal(e)
	}
}
func p2Item(key string, n int32) *pb.BaseResource {
	return &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(n)}
}
func p2Recruit(n int32, free int32) *pb.CSWarriorRecruit {
	return &pb.CSWarriorRecruit{CostType: proto.Int32(3), NumType: proto.Int32(n), Free: proto.Int32(free)}
}

// 原ID、原资质、空间过滤与本地保底范围必须一致；重启保留运营调整。
func TestP2KingIdentityAndRotation(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-identity", 20)
	_, rows, e := s.loadGameplayPool(db.GetIns(), "recruit_3")
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		h := s.table("Hero", "id", r.ResourceID)
		if h["heroClass"] != "0" || (h["quality"] != "20" && h["quality"] != "23") || r.Guaranteed != (h["quality"] == "23") {
			t.Fatal("identity", r, h)
		}
	}
	for cycle := int64(0); cycle < 6; cycle++ {
		var rule db.KingRecruitRules
		db.GetIns().First(&rule, 1)
		now := time.UnixMilli(rule.EpochMillis + cycle*rule.RotationMillis)
		_, space, next, e := s.kingConfiguration(db.GetIns(), now)
		if e != nil || space != int32(cycle%3+1) || next != rule.EpochMillis+(cycle+1)*rule.RotationMillis {
			t.Fatal("rotation", space, next, e)
		}
	}
	messages, e := s.recruitInfo(ctx, id, &pb.CSWarriorRecruitGet{})
	if e != nil {
		t.Fatal(e)
	}
	all := messages[0].(*pb.SCWarriorRecruitGet).GetRecruit()
	if len(all) != 3 {
		t.Fatal(all)
	}
	king := all[2]
	if king.GetOn() != 1 || king.GetNum() != 50 || king.GetLeftTimes() != 9 || king.GetLuck() != 0 || !king.GetFree() {
		t.Fatal(king)
	}
	preview, e := s.kingRecruitPreview(ctx, id, &pb.CSWarriorRecruitGetPreview{CostType: proto.Int32(3)})
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range preview[0].(*pb.SCWarriorRecruitGetPreview).GetRewards() {
		if s.table("Hero", "id", key)["space"] != fmt.Sprint(king.GetSpace()) {
			t.Fatal("preview crosses rotation", key)
		}
	}
	db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=?", "recruit_3").Update("weight", 123)
	if e = s.InitializeP2(ctx); e != nil {
		t.Fatal(e)
	}
	var r db.GameplayPoolReward
	db.GetIns().First(&r, "pool_id=?", "recruit_3")
	if r.Weight != 123 {
		t.Fatal("restart replaced SQL", r)
	}
}

func TestP2KingDrawGatePityQuotaReplay(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-draw", 19)
	p2Funding(t, s, ctx, id, "diamond,20000")
	if _, e := s.warriorRecruit(ctx, id, p2Recruit(1, 0), 1); e == nil {
		t.Fatal("gate")
	}
	if e := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		recruitState(g, 3, time.Now()).SinceGuarantee = 9
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	before, _ := p2Profile(t, id)
	messages, e := s.warriorRecruit(ctx, id, p2Recruit(1, 0), 2)
	if e != nil {
		t.Fatal(e)
	}
	result := messages[0].(*pb.SCWarriorRecruit)
	if len(result.GetGet().GetWars()) != 1 || s.table("Hero", "id", result.GetGet().GetWars()[0].GetId())["quality"] != "23" {
		t.Fatal("pity failed", result)
	}
	retry, e := s.warriorRecruit(ctx, id, p2Recruit(1, 0), 2)
	if e != nil || !proto.Equal(result, retry[0]) {
		t.Fatal("replay", e)
	}
	if _, e = s.warriorRecruit(ctx, id, p2Recruit(2, 0), 2); e == nil {
		t.Fatal("timestamp conflict")
	}
	p, g := p2Profile(t, id)
	if p.GetDiamond() != before.GetDiamond()-300 || p.GetCoin() != before.GetCoin()+10000 || g.Recruit[3].SinceGuarantee != 0 {
		t.Fatal("wallet/pity", p, g.Recruit[3])
	}
	messages, e = s.warriorRecruit(ctx, id, p2Recruit(2, 0), 3)
	if e != nil {
		t.Fatal(e)
	}
	if len(messages[0].(*pb.SCWarriorRecruit).GetGet().GetWars()) != 10 {
		t.Fatal("ten")
	}
	p, _ = p2Profile(t, id)
	if p.GetDiamond() != before.GetDiamond()-3100 {
		t.Fatal("ten cost", p)
	}
	var st db.KingRecruitState
	db.GetIns().First(&st, id)
	_, saved := p2Profile(t, id)
	if st.Draws != 11 || *saved.KingRecruitSharedLuck != 110 {
		t.Fatal(st)
	}
	st.Draws = 49
	db.GetIns().Save(&st)
	if _, e = s.warriorRecruit(ctx, id, p2Recruit(2, 0), 4); e == nil {
		t.Fatal("quota accepted ten")
	}
	if _, e = s.warriorRecruit(ctx, id, p2Recruit(1, 1), 5); e != nil {
		t.Fatal(e)
	}
	if _, e = s.warriorRecruit(ctx, id, p2Recruit(1, 0), 6); e == nil {
		t.Fatal("quota ignored")
	}
	db.GetIns().First(&st, id)
	st.Day = "2000-01-01"
	db.GetIns().Save(&st)
	if _, e = s.warriorRecruit(ctx, id, p2Recruit(1, 1), 7); e == nil {
		t.Fatal("daily reset must not reset 48h free")
	}
	if _, e = s.warriorRecruit(ctx, id, p2Recruit(1, 0), 8); e != nil {
		t.Fatal("paid day reset", e)
	}
	db.GetIns().First(&st, id)
	_, saved = p2Profile(t, id)
	if st.Draws != 1 || *saved.KingRecruitSharedLuck != 130 {
		t.Fatal(st)
	}
}

func TestP2KingInvalidSQLRollsBack(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-config", 20)
	p2Funding(t, s, ctx, id, "diamond,10000")
	p, g := p2Profile(t, id)
	for _, change := range []map[string]any{{"single_cost": "diamond,1"}, {"min_level": 1}, {"free_seconds": 1}} {
		var original db.GameplayPool
		db.GetIns().First(&original, "id=?", "recruit_3")
		db.GetIns().Model(&original).Updates(change)
		if _, e := s.warriorRecruit(ctx, id, p2Recruit(1, 0), 1); e == nil {
			t.Fatal("accepted invalid client rule", change)
		}
		db.GetIns().Save(&original)
	}
	// 运行表非法时同样在扣费前失败；恢复后原回执与进度保持。
	db.GetIns().Model(&db.RecruitRuntimeRule{}).Where("kind=?", 3).Update("guarantee_every", 0)
	if _, e := s.warriorRecruit(ctx, id, p2Recruit(1, 0), 1); e == nil {
		t.Fatal("daily mismatch")
	}
	db.GetIns().Model(&db.RecruitRuntimeRule{}).Where("kind=?", 3).Update("guarantee_every", 10)
	now, game := p2Profile(t, id)
	if !proto.Equal(p, now) || len(game.Warriors) != len(g.Warriors) {
		t.Fatal("failed draw mutated profile")
	}
}

func TestP2LuckyRedSelectionDurable(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-luck", 20)
	rules, space, _, e := s.kingConfiguration(db.GetIns(), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	_, rows, e := s.loadGameplayPool(db.GetIns(), "recruit_3")
	if e != nil {
		t.Fatal(e)
	}
	_, red := s.kingRewards(rows, space)
	state, e := kingState(db.GetIns(), id, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	state.Luck = rules.LuckLimit
	db.GetIns().Save(state)
	bad := &pb.CSWarriorRecruitLuckyReward{Reward: proto.String("hero_046")}
	if _, e = s.kingRecruitLucky(ctx, id, bad, 100); e == nil {
		t.Fatal("avatar accepted")
	}
	request := &pb.CSWarriorRecruitLuckyReward{Reward: proto.String(red[0])}
	first, e := s.kingRecruitLucky(ctx, id, request, 100)
	if e != nil {
		t.Fatal(e)
	}
	if len(first[0].(*pb.SCWarriorRecruitLuckyReward).GetGet().GetWars()) != 1 {
		t.Fatal(first)
	}
	second, e := s.kingRecruitLucky(ctx, id, request, 100)
	if e != nil || !proto.Equal(first[0], second[0]) {
		t.Fatal("durable lucky replay", e)
	}
	if _, e = s.kingRecruitLucky(ctx, id, request, 101); e == nil {
		t.Fatal("luck charged twice")
	}
	db.GetIns().First(state, id)
	_, saved := p2Profile(t, id)
	if *saved.KingRecruitSharedLuck != 0 {
		t.Fatal(state)
	}
}

func TestP2BoxesOriginalIndicesAndRollback(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-box", 1)
	p2Funding(t, s, ctx, id, "item_023,1;item_053,2;item_119,2;item_361,2")
	use := func(key string, n, index int32, stamp int64) ([]proto.Message, error) {
		return s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item(key, n), GetItemIndex: proto.Int32(index)}, stamp)
	}
	if _, e := use("item_023", 1, 0, 1); e == nil {
		t.Fatal("zero row index accepted")
	}
	first, e := use("item_023", 1, 5, 2)
	if e != nil {
		t.Fatal(e)
	}
	if first[0].(*pb.SCItemUse).GetGet().GetSimps()[0].GetId() != "item_053" {
		t.Fatal(first)
	}
	replay, e := use("item_023", 1, 5, 2)
	if e != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("box replay", e)
	}
	heroes, e := use("item_053", 2, 1, 3)
	if e != nil {
		t.Fatal(e)
	}
	wars := heroes[0].(*pb.SCItemUse).GetGet().GetWars()
	if len(wars) != 2 || wars[0].GetId() != "hero_001" || wars[0].GetGuid() == wars[1].GetGuid() {
		t.Fatal("box instances", wars)
	}
	equipment, e := use("item_119", 1, 1, 4)
	if e != nil {
		t.Fatal(e)
	}
	equips := equipment[0].(*pb.SCItemUse).GetGet().GetEquips()
	if len(equips) != 1 || equips[0].GetEquipId() != "equip_037" {
		t.Fatal(equipment)
	}
	random, e := use("item_361", 2, 0, 5)
	if e != nil {
		t.Fatal(e)
	}
	if len(random[0].(*pb.SCItemUse).GetGet().GetWars()) != 2 {
		t.Fatal("random box must give single candidate per box", random)
	}
	p, g := p2Profile(t, id)
	if e = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.EquipBagSize = proto.Int32(int32(len(g.Equipment)))
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if _, e = use("item_119", 1, 1, 6); e == nil {
		t.Fatal("full equipment bag")
	}
	p2, g2 := p2Profile(t, id)
	if simpleOwned(p2, g2, "item_119") != simpleOwned(p, g, "item_119") || len(g2.Equipment) != len(g.Equipment) {
		t.Fatal("bag failure consumed box")
	}
	var count int64
	db.GetIns().Model(&db.RedP2Receipt{}).Where("player_id=? AND timestamp=?", id, 6).Count(&count)
	if count != 0 {
		t.Fatal("failed receipt persisted")
	}
}

func TestP2FragmentsCombineAndSell(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-frag", 1)
	frag := s.table("Frag", "target", "hero_001")
	n := int32(tableNumber(frag, "num"))
	p2Funding(t, s, ctx, id, fmt.Sprintf("%s,%d;item_122,1", frag["id"], n+2))
	selected, e := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item("item_122", 1), GetItemIndex: proto.Int32(1)}, 1)
	if e != nil || selected[0].(*pb.SCItemUse).GetGet().GetSimps()[0].GetId() != frag["id"] {
		t.Fatal("fragment box", e)
	}
	queried, e := s.redFragGet(ctx, id, &pb.CSWarriorFragGet{})
	if e != nil || len(queried[0].(*pb.SCWarriorFragGet).GetWarriorFrag()) != 1 {
		t.Fatal(queried, e)
	}
	request := &pb.CSCombine{TargetId: proto.String("hero_001")}
	first, e := s.redCombine(ctx, id, request, 2)
	if e != nil {
		t.Fatal(e)
	}
	repeat, e := s.redCombine(ctx, id, request, 2)
	if e != nil || !proto.Equal(first[0], repeat[0]) {
		t.Fatal("combine replay", e)
	}
	if first[0].(*pb.SCCombine).GetCost()[0].GetNum() != n {
		t.Fatal(first)
	}
	before, g := p2Profile(t, id)
	owned := int32(simpleOwned(before, g, frag["id"]))
	beforeSoul := simpleOwned(before, g, "item_033")
	sale := &pb.CSWarriorFragSell{Sell: []*pb.BaseResource{p2Item(frag["id"], owned)}}
	sold, e := s.redFragSell(ctx, id, sale, 3)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.redFragSell(ctx, id, sale, 3)
	if e != nil || !proto.Equal(sold[0], retry[0]) {
		t.Fatal("sale replay", e)
	}
	after, g := p2Profile(t, id)
	if after.GetCoin() != before.GetCoin() || simpleOwned(after, g, "item_033") != beforeSoul+int64(owned)*int64(tableNumber(frag, "price")) || simpleOwned(after, g, frag["id"]) != 0 {
		t.Fatal("sale wallet")
	}
}

func TestP2ExchangeCatalogQuotaAndPrice(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-exchange", 20)
	p2Funding(t, s, ctx, id, "item_020,800;item_021,200")
	request := &pb.CSGetActivityShopReward{ActivityId: proto.String(constUtil.P2RedExchangeActivity), Index: proto.Int32(3), CostResources: []*pb.BaseResource{p2Item("item_021", 99)}}
	if _, e := s.redExchangeBuy(ctx, id, request, 1); e == nil {
		t.Fatal("spoofed price")
	}
	request.CostResources = []*pb.BaseResource{p2Item("item_021", 100)}
	request.GetResources = []*pb.BaseResource{p2Item("equip_041", 1)}
	if _, e := s.redExchangeBuy(ctx, id, request, 1); e == nil {
		t.Fatal("spoofed item")
	}
	request.GetResources = []*pb.BaseResource{p2Item("equip_037", 1)}
	first, e := s.redExchangeBuy(ctx, id, request, 1)
	if e != nil {
		t.Fatal(e)
	}
	if first[0].(*pb.SCGetActivityShopReward).GetGet().GetEquips()[0].GetEquipId() != "equip_037" {
		t.Fatal(first)
	}
	retry, e := s.redExchangeBuy(ctx, id, request, 1)
	if e != nil || !proto.Equal(first[0], retry[0]) {
		t.Fatal("exchange replay", e)
	}
	if _, e = s.redExchangeBuy(ctx, id, request, 2); e == nil {
		t.Fatal("daily quota")
	}
	p, g := p2Profile(t, id)
	if simpleOwned(p, g, "item_021") != 100 || len(g.Equipment) != 1 {
		t.Fatal("duplicate exchange")
	}
	catalog, e := s.redExchangeInfo(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	for _, item := range catalog[0].(*pb.SCActivityShopInfo).GetShopItemList() {
		if item.GetIndex() == 3 && item.GetLeft() != 0 {
			t.Fatal("left counter")
		}
	}
}

func TestP2RedGrowthFormationAndBattle(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-grow", 30)
	p2Funding(t, s, ctx, id, "item_053,1;item_119,1;coin,1000000")
	h, e := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item("item_053", 1), GetItemIndex: proto.Int32(1)}, 1)
	if e != nil {
		t.Fatal(e)
	}
	hero := h[0].(*pb.SCItemUse).GetGet().GetWars()[0]
	gear, e := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item("item_119", 1), GetItemIndex: proto.Int32(1)}, 2)
	if e != nil {
		t.Fatal(e)
	}
	equip := gear[0].(*pb.SCItemUse).GetGet().GetEquips()[0]
	if _, e = s.formationOn(ctx, id, &pb.CSFormationOn{Id: hero.Id, Guid: hero.Guid, FormatPos: proto.Int32(2)}); e != nil {
		t.Fatal(e)
	}
	p, g := p2Profile(t, id)
	var slot *pb.FormationItem
	for _, f := range g.Formation {
		if f.GetFormatPos() == 2 {
			slot = f
		}
	}
	base, e := s.battleWarrior(p, hero, g, slot)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.equipmentFormationOn(ctx, id, &pb.CSFormationOn{Id: equip.EquipId, Guid: equip.Guid, FormatPos: proto.Int32(2)}); e != nil {
		t.Fatal(e)
	}
	p, g = p2Profile(t, id)
	for _, f := range g.Formation {
		if f.GetFormatPos() == 2 {
			slot = f
		}
	}
	after, e := s.battleWarrior(p, hero, g, slot)
	if e != nil {
		t.Fatal(e)
	}
	if after.GetProp().GetAtt() <= base.GetProp().GetAtt() {
		t.Fatal("red gear missing battle", base, after)
	}
	cost, e := s.warriorLevelExperience("23", 1)
	if e != nil || cost < 1 {
		t.Fatal("original red exp", cost, e)
	}
	if s.table("Hero", "id", hero.GetId())["quality"] != "23" || s.table("Equip", "id", equip.GetEquipId())["quality"] != "23" {
		t.Fatal("wrong identity")
	}
}

// 原红色档位经过实际培养业务；高阶入口保留原等级门槛。
func TestP2OriginalRedCultivation(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-cultivate", 70)
	if e := s.InitEquipmentMutations(ctx); e != nil {
		t.Fatal(e)
	}
	p2Funding(t, s, ctx, id, "item_053,1;item_119,1;coin,1000000;hero_exp_003,1;item_008,10")
	heroes, e := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item("item_053", 1), GetItemIndex: proto.Int32(1)}, 1)
	if e != nil {
		t.Fatal(e)
	}
	hero := heroes[0].(*pb.SCItemUse).GetGet().GetWars()[0]
	gear, e := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item("item_119", 1), GetItemIndex: proto.Int32(1)}, 2)
	if e != nil {
		t.Fatal(e)
	}
	equip := gear[0].(*pb.SCItemUse).GetGet().GetEquips()[0]
	_, g := p2Profile(t, id)
	var material *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_exp_003" {
			material = w
		}
	}
	grown, e := s.warriorStrengthen(ctx, id, &pb.CSWarriorStrengthen{Guid: hero.Guid, Cost: []*pb.BaseResource{{Id: material.Id, Guid: material.Guid}}})
	if e != nil {
		t.Fatal(e)
	}
	if grown[0].(*pb.SCWarriorStrengthen).GetGuid().GetLevel() <= 1 {
		t.Fatal("red hero did not grow")
	}
	var cost int32
	for _, row := range s.Tables["EquipExp"] {
		if row["quality"] == "23" && row["level"] == "1" {
			cost = int32(tableNumber(row, "coin"))
		}
	}
	strengthened, e := s.equipmentStrengthen(ctx, id, &pb.CSEquipStrengthen{Guid: equip.Guid, Times: proto.Int32(1), Cost: []*pb.BaseResource{p2Item("coin", cost)}})
	if e != nil {
		t.Fatal(e)
	}
	if strengthened[0].(*pb.SCEquipStrengthen).GetEquip().GetLevel() != 2 {
		t.Fatal("red equip strength")
	}
	refined, e := s.equipmentRefine(ctx, id, &pb.CSEquipRefine{Guid: equip.Guid, Cost: []*pb.BaseResource{p2Item("item_008", 10)}}, 3)
	if e != nil {
		t.Fatal(e)
	}
	if refined[0].(*pb.SCEquipRefine).GetEquip().GetRefineLv() < 1 {
		t.Fatal("red refine")
	}
	starCost := s.equipmentStarRow("1", 1)["cost"]
	p2Funding(t, s, ctx, id, starCost)
	starred, e := s.equipmentStar(ctx, id, &pb.CSEquipStar{Guid: equip.Guid}, 4)
	if e != nil {
		t.Fatal(e)
	}
	if starred[0].(*pb.SCEquipStar).GetEquip().GetStar() != 1 {
		t.Fatal("red star")
	}
	_, g = p2Profile(t, id)
	owned, e := s.equipmentRecord(g, equip.GetGuid())
	if e != nil {
		t.Fatal(e)
	}
	costs, _, e := s.equipmentForgeCosts(owned)
	if e != nil {
		t.Fatal(e)
	}
	for _, cost := range costs {
		p2Funding(t, s, ctx, id, fmt.Sprintf("%s,%d", cost.GetId(), cost.GetNum()))
	}
	_, e = s.equipmentForge(ctx, id, &pb.CSEquipForge{Guid: equip.Guid, Cost: costs}, 5)
	if e != nil {
		t.Fatal(e)
	}
	// 锻造有原概率，验收扣费/回执/合法存档，不假设随机成功。
	p, g := p2Profile(t, id)
	if _, e = s.equipmentRecord(g, equip.GetGuid()); e != nil {
		t.Fatal(e)
	}
	if p.GetCoin() >= 1000000+int64(costs[len(costs)-1].GetNum()) {
		t.Fatal("cultivation did not charge")
	}
}

func TestP2OldTurntableSettlementSurvivesSwitch(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "p2-old-pool", 20)
	p2Funding(t, s, ctx, id, "diamond,1000")
	db.GetIns().Model(&db.KingRecruitRules{}).Where("id=1").Update("wheel_pool", "turntable_20261011")
	if _, e := s.playBigTurn(ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 1); e != nil {
		t.Fatal(e)
	}
	db.GetIns().Model(&db.GameplayPool{}).Where("id=?", "turntable_20261011").Update("end_at", time.Now().UnixMilli()-1)
	db.GetIns().Model(&db.KingRecruitRules{}).Where("id=1").Update("wheel_pool", constUtil.P2RedWheelPool)
	p, g := p2Profile(t, id)
	before := simpleOwned(p, g, "item_008")
	if e := s.SettleTurntable(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.SettleTurntable(ctx); e != nil {
		t.Fatal(e)
	}
	p, g = p2Profile(t, id)
	if simpleOwned(p, g, "item_008") != before+100 {
		t.Fatal("old-pool ranking lost or doubled")
	}
}

// MySQL 中实际锁定同一玩家及共享兑换库存，验证并发消息和实例回执。
func TestMySQLP2Transactions(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p2_") {
		t.Skip("isolated P2 MySQL database required")
	}
	ctx := context.Background()
	if e := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}); e != nil {
		t.Fatal(e)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	s := &GameService{}
	if e := s.LoadTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	initializeP2Test(t, s, ctx)
	var ids []int64
	for i := 0; i < 2; i++ {
		id := dailyCopyTestPlayer(t, ctx, fmt.Sprintf("p2-mysql-%d-%d", time.Now().UnixNano(), i), 20)
		p2Funding(t, s, ctx, id, "item_119,1;item_021,200")
		ids = append(ids, id)
	}
	request := &pb.CSItemUse{CostItem: p2Item("item_119", 1), GetItemIndex: proto.Int32(1)}
	var wg sync.WaitGroup
	results := make(chan proto.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, e := s.itemUse(ctx, ids[0], request, 1)
			errs <- e
			if e == nil {
				results <- messages[0]
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first proto.Message
	for result := range results {
		if first == nil {
			first = result
		} else if !proto.Equal(first, result) {
			t.Fatal("concurrent box produced different instances")
		}
	}
	_, g := p2Profile(t, ids[0])
	if len(g.Equipment) != 1 {
		t.Fatal("box repeated", len(g.Equipment))
	}
	db.GetIns().Model(&db.RedP2Exchange{}).Where("id=3").Updates(map[string]any{"stock": 1, "used": 0})
	outcomes := make(chan error, 2)
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, e := s.redExchangeBuy(ctx, id, &pb.CSGetActivityShopReward{ActivityId: proto.String(constUtil.P2RedExchangeActivity), Index: proto.Int32(3)}, 2)
			outcomes <- e
		}(id)
	}
	wg.Wait()
	close(outcomes)
	success := 0
	for e := range outcomes {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("global stock race", success)
	}
	var cfg db.RedP2Exchange
	db.GetIns().First(&cfg, 3)
	if cfg.Used != 1 {
		t.Fatal("global stock", cfg)
	}
	if e := s.InitializeP2(ctx); e != nil {
		t.Fatal(e)
	}
	again, e := s.itemUse(ctx, ids[0], request, 1)
	if e != nil || !proto.Equal(first, again[0]) {
		t.Fatal("restart receipt", e)
	}
}

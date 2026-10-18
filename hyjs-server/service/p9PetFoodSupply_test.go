package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p9FoodSupplyFixture(t *testing.T) (*p1Fixture, string, time.Time) {
	t.Helper()
	// Bind the real wallet calendar owner so rewinding restores persisted daily quota.
	f, _ := localPlayActivitiesFixture(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP9Pets, f.s.InitializeP9PetNext, f.s.InitializeP9PetRecycle, f.s.InitializeP9CopyIntimacy, f.s.InitializeP9PetIntimacy} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := p4At(3, 12)
	if err := f.s.seedP9PetFoodSupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	return f, constUtil.P9PetFoodSupplyPrefix + now.Format("200601"), now
}

// Only level/coin are finite fixture qualifications. The real original first
// recruit and two days of paid source purchases produce every pet/food instance.
func TestP9PetFoodSupplyActualFirstPetPaidFeedTalentAndCold(t *testing.T) {
	f, key, now := p9FoodSupplyFixture(t)
	var rule db.P4CommerceRule
	if err := db.GetIns().First(&rule, "id=?", key).Error; err != nil || rule.Enabled || len(rule.Frozen) != 0 {
		t.Fatal("source is not initially closed", err)
	}
	products, err := f.s.p9PetFoodProducts()
	if err != nil || len(products) != 1 || products[0].ID != "item_129" || products[0].Cost != "coin,60000" || products[0].Rewards != "item_129,6" || products[0].MinLevel != 34 || products[0].DailyLimit != 5 || products[0].TotalLimit != 150 {
		t.Fatal("named source or original food changed", products, err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now); err == nil {
		t.Fatal("unpublished supply opened")
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level, p.Coin = proto.Int32(33), proto.Int64(1000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&rule).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	outer := p4ExchangeKey(now)
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(outer)}, 0, now).(*pb.SCActivityShopInfo)
	var quote *pb.ActivityShopItem
	for _, item := range listing.GetShopItemList() {
		if len(item.GetGetResource()) == 1 && item.GetGetResource()[0].GetId() == "item_129" {
			quote = item
		}
	}
	if quote == nil {
		t.Fatal("new global address missing")
	}
	buy := &pb.CSBuyActivityShop{ActivityId: proto.String(outer), Index: quote.Index, Num: proto.Int32(5), CostResources: quote.CostResource, GetResources: quote.GetResource}
	if _, err = f.s.p4Commerce(f.ctx, f.id, buy, 998100, now); err == nil {
		t.Fatal("original34 gate bypassed")
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(34); return nil }); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.p9PetFirst(f.ctx, f.id, 998101, &pb.CSPetFirstRecruit{})
	if err != nil {
		t.Fatal(err)
	}
	pet := first[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	if pet.GetId() != "pet_002" {
		t.Fatal("original first recruit changed")
	}
	purchased := p4Trade(t, f, buy, 998102, now)
	if !proto.Equal(purchased, p4Trade(t, f, buy, 998102, now)) {
		t.Fatal("paid source replay changed")
	}
	extra := proto.Clone(buy).(*pb.CSBuyActivityShop)
	extra.Num = proto.Int32(1)
	if _, err = f.s.p4Commerce(f.ctx, f.id, extra, 998103, now); err == nil {
		t.Fatal("daily five limit bypassed")
	}
	p4Trade(t, f, extra, 998104, now.AddDate(0, 0, 1))
	if _, err = f.s.p4Commerce(f.ctx, f.id, extra, 998105, now); err == nil {
		t.Fatal("rewind restored old daily allowance")
	}
	feed := &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(32)}}}
	fed, err := f.s.p9PetEat(f.ctx, f.id, feed, 998106)
	if err != nil {
		t.Fatal(err)
	}
	current := fed[0].(*pb.SCPetEat).GetPet()
	if current.GetEat().GetLevel() != 1 || current.GetEat().GetExp() != 14 || len(current.GetRandPropList()) != 1 || current.GetRandPropList()[0] == current.GetInitProp() {
		t.Fatal("actual32 original same-nature foods did not unlock the original626 threshold", current)
	}
	beforeProperties, beforeTalents, err := f.s.p9PetProperties(pet)
	if err != nil {
		t.Fatal(err)
	}
	afterProperties, afterTalents, err := f.s.p9PetProperties(current)
	if err != nil || (reflect.DeepEqual(beforeProperties, afterProperties) && reflect.DeepEqual(beforeTalents, afterTalents)) {
		t.Fatal("real allocated talent has no derived consumer", err)
	}
	p, g := f.profile(t)
	if p.GetCoin() != 640000 || simpleOwned(p, g, "item_129") != 4 {
		t.Fatal("source or feed was not charged exactly once")
	}
	before, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	replayed, err := f.s.p9PetEat(f.ctx, f.id, feed, 998106)
	if err != nil || !proto.Equal(replayed[0], fed[0]) {
		t.Fatal("cold feed receipt changed", err)
	}
	p, g = f.profile(t)
	after, err := json.Marshal(g)
	if err != nil || p.GetCoin() != 640000 || string(before) != string(after) {
		t.Fatal("cold pet or food changed")
	}
}

func TestP9PetFoodSupplyPrivateRefreshKeepsOriginalDirectory(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	now := LocalPlayNow(f.ctx)
	tx := db.GetIns().WithContext(f.ctx)
	if err := f.s.seedP9PetFoodSupply(tx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitExchange", "99820000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	key, err := localPlayExchangeKey(tx, now)
	if err != nil {
		t.Fatal(err)
	}
	directory, bindings, err := f.s.p4ExchangeDirectory(tx, key, now)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash, beforeRaw := directory.InputHash, string(directory.Bindings)
	global := constUtil.P9PetFoodSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	if err = tx.Model(&db.P4CommerceRule{}).Where("id=?", global).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.p9AppendPetFoodBinding(tx, directory, &bindings, now); err != nil || directory.InputHash != beforeHash || string(directory.Bindings) != beforeRaw {
		t.Fatal("late source mutated private frozen directory", err)
	}
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "TimeLimitExchange", "99820000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	freshKey, err := localPlayExchangeKey(tx, now)
	if err != nil || freshKey == key {
		t.Fatal("private refresh identity", err)
	}
	_, fresh, err := f.s.p4ExchangeDirectory(tx, freshKey, now)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, binding := range fresh {
		if binding.ProductID == "item_129" && binding.ChildID != global {
			count++
		}
	}
	if count != 1 {
		t.Fatal("fresh private cycle did not adopt the single source", count)
	}
	var old db.P4ResourceExchangeDirectory
	if err = tx.First(&old, "id=?", key).Error; err != nil || old.InputHash != beforeHash || string(old.Bindings) != beforeRaw {
		t.Fatal("old private directory history changed", err)
	}
}

func TestP9PetFoodSupplyFrozenMonthlyPublicationAndShutdown(t *testing.T) {
	f, key, now := p9FoodSupplyFixture(t)
	var rule db.P4CommerceRule
	if err := db.GetIns().First(&rule, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(rule.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.Products[0].Cost = "coin,70000"
	rule.Rules, _ = json.Marshal(rules)
	rule.Enabled = true
	rule.Frozen, _ = json.Marshal(rule)
	if err := db.GetIns().Save(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&rule).Update("rules", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	next := now.AddDate(0, 1, 0)
	if err := f.s.seedP9PetFoodSupply(db.GetIns(), next); err != nil {
		t.Fatal(err)
	}
	var inherited db.P4CommerceRule
	if err := db.GetIns().First(&inherited, "id=?", constUtil.P9PetFoodSupplyPrefix+next.Format("200601")).Error; err != nil {
		t.Fatal(err)
	}
	_, actual, err := f.s.p9PetFoodSnapshot(inherited)
	if err != nil || !inherited.Enabled || actual.Products[0].Cost != "coin,70000" {
		t.Fatal("frozen legal quote/publication not inherited", err)
	}
	if err = db.GetIns().Model(&inherited).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{next, next.AddDate(0, 1, 0)} {
		if err = f.s.seedP9PetFoodSupply(db.GetIns(), at); err != nil {
			t.Fatal(err)
		}
		var current db.P4CommerceRule
		if err = db.GetIns().First(&current, "id=?", constUtil.P9PetFoodSupplyPrefix+at.Format("200601")).Error; err != nil || current.Enabled {
			t.Fatal("restart/monthly seed undid operator shutdown", err)
		}
	}
}

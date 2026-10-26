package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p5AwakeElementFixture(t *testing.T) (*p1Fixture, string, time.Time) {
	t.Helper()
	f := p4Setup(t)
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeWarriorAwake, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	now := p4At(3, 12) // Finite service clock, two days strictly within one month.
	if err := f.s.seedP5AwakeElementSupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	key := constUtil.P5AwakeElementSupplyPrefix + now.Format("200601")
	return f, key, now
}

func TestP5AwakeElementSupplyOriginalChoicesAndClosedCatalog(t *testing.T) {
	f, key, now := p5AwakeElementFixture(t)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || len(cfg.Frozen) != 0 || cfg.Source != constUtil.P5AwakeElementSupplySource {
		t.Fatal("unpublished source must remain closed/unfrozen")
	}
	products, err := f.s.p5AwakeElementProducts()
	if err != nil || len(products) != 1 {
		t.Fatal(products, err)
	}
	p := products[0]
	if p.ID != "item_337" || p.Rewards != "item_337,100" || p.Cost != "coin,500000" || p.MinLevel != 70 || p.DailyLimit != 5 || p.TotalLimit != 150 {
		t.Fatal("named local source changed", p)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now); err == nil {
		t.Fatal("unpublished source visible")
	}
	var directory db.P4ResourceExchangeDirectory
	directory.ID = "limitExchange_p4_lp-00000000000000000000000000"
	directory.Bindings, directory.InputHash = []byte("[]"), "unchanged"
	bindings := []p4ExchangeBinding{}
	if err = f.s.p5AppendAwakeElementBinding(db.GetIns(), &directory, &bindings, now); err != nil || directory.InputHash != "unchanged" || string(directory.Bindings) != "[]" || len(bindings) != 0 {
		t.Fatal("private frozen directory touched", err)
	}
	// All five original choices must remain validated, including outside the
	// current 001/003 pair. A changed recipe is rejected, never repaired here.
	for _, option := range f.s.Tables["ItemUse"] {
		if option["itemId"] == "item_337" && option["odds"] == "3" {
			original := option["result"]
			option["result"] = "item_322,2"
			if _, err = f.s.p5AwakeElementProducts(); err == nil {
				t.Fatal("altered original choice accepted")
			}
			option["result"] = original
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level, p.Coin = proto.Int32(70), proto.Int64(1000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&cfg).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	listed := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	quote := listed.ShopItemList[0]
	p4Trade(t, f, &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: quote.Index, Num: proto.Int32(1), CostResources: quote.CostResource, GetResources: quote.GetResource}, 99601, now)
	for i, id := range []string{"item_320", "item_321", "item_322", "item_323", "item_324"} {
		used, err := f.s.p5BoxUse(f.ctx, f.id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_337"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(int32(i + 1))}, int64(99610+i))
		if err != nil {
			t.Fatal(err)
		}
		gain := used[0].(*pb.SCItemUse).GetGet().GetSimps()
		if len(gain) != 1 || gain[0].GetId() != id || gain[0].GetNum() != 1 {
			t.Fatal("original choice changed", i, gain)
		}
	}
}

// No337 or322 is injected. Qualification capital, level, three idle original
// hero003 bodies, talent material and the other two awakening materials are
// explicit finite fixtures; this is not a fresh natural whole-awakening proof.
func TestP5AwakeElementSupplyPurchaseChoiceActual003AwakeAndCold(t *testing.T) {
	f, key, now := p5AwakeElementFixture(t)
	var target string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin = proto.Int32(69), proto.Int64(10000000)
		gain, err := f.s.GrantFixedReward(p, g, "hero_003,3;item_005,30;item_315,50;item_312,15")
		if err != nil {
			return err
		}
		target = gain.Wars[0].GetGuid()
		w, err := p5Warrior(g, target)
		if err == nil {
			w.Level = proto.Int32(70)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	base := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(p4ExchangeKey(now))}, 0, now).(*pb.SCActivityShopInfo)
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", key).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	list := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(p4ExchangeKey(now))}, 0, now).(*pb.SCActivityShopInfo)
	if len(list.ShopItemList) != len(base.ShopItemList)+1 {
		t.Fatal("late source not appended once")
	}
	for i, item := range base.ShopItemList {
		if !proto.Equal(item, list.ShopItemList[i]) {
			t.Fatal("old outer index changed", i)
		}
	}
	quote := list.ShopItemList[len(list.ShopItemList)-1]
	buy := &pb.CSBuyActivityShop{ActivityId: proto.String(p4ExchangeKey(now)), Index: quote.Index, Num: proto.Int32(5), CostResources: quote.CostResource, GetResources: quote.GetResource}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 99701, now); err == nil {
		t.Fatal("original70 level qualification bypass")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(70); return nil }); err != nil {
		t.Fatal(err)
	}
	first := p4Trade(t, f, buy, 99702, now)
	if !proto.Equal(first, p4Trade(t, f, buy, 99702, now)) {
		t.Fatal("purchase replay changed")
	}
	excess := proto.Clone(buy).(*pb.CSBuyActivityShop)
	excess.Num = proto.Int32(1)
	if _, err := f.s.p4Commerce(f.ctx, f.id, excess, 99703, now); err == nil {
		t.Fatal("daily5 cap bypass")
	}
	second := proto.Clone(buy).(*pb.CSBuyActivityShop)
	second.Num = proto.Int32(3)
	p4Trade(t, f, second, 99704, now.AddDate(0, 0, 1))
	p, g := f.profile(t)
	if p.GetCoin() != 6000000 || simpleOwned(p, g, "item_337") != 800 || simpleOwned(p, g, "item_322") != 0 {
		t.Fatal("paid source exact amount")
	}
	for option := int32(0); option <= 6; option += 6 {
		bad := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_337"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(option)}
		if _, err := f.s.p5BoxUse(f.ctx, f.id, bad, int64(99710+option)); err == nil {
			t.Fatal("non-original choice accepted", option)
		}
	}
	for i := 0; i < 8; i++ {
		use := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_337"), Num: proto.Int32(100)}, GetItemIndex: proto.Int32(3)}
		out, err := f.s.p5BoxUse(f.ctx, f.id, use, int64(99720+i))
		if err != nil {
			t.Fatal(err)
		}
		var gained int32
		for _, item := range out[0].(*pb.SCItemUse).GetGet().GetSimps() {
			if item.GetId() != "item_322" {
				t.Fatal("selected original element changed")
			}
			gained += item.GetNum()
		}
		if gained != 100 {
			t.Fatal("original five-choice amount changed")
		}
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_337") != 0 || simpleOwned(p, g, "item_322") != 800 {
		t.Fatal("original choice not credited")
	}
	actualContext := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	talent := &pb.CSWarriorTalent{Guid: proto.String(target), Talent: proto.String("1"), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(10000)}, {Id: proto.String("item_005"), Num: proto.Int32(30)}}}
	if _, err := f.s.warriorTalent(actualContext(talent), f.id, talent); err != nil {
		t.Fatal(err)
	}
	awake := &pb.CSWarriorAwake{Guid: proto.String(target)}
	result, err := f.s.warriorAwake(actualContext(awake), f.id, awake, 99740)
	if err != nil || result[0].(*pb.SCWarriorAwake).GetNewWarrior().GetAwake().GetStage() != 1 {
		t.Fatal("actual new800 element consumer", err)
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_322") != 0 || simpleOwned(p, g, "item_315") != 0 || simpleOwned(p, g, "item_312") != 0 {
		t.Fatal("original awakening recipe not consumed")
	}
	beforeP, beforeG := proto.Clone(p).(*pb.Player), p5AwakeElementJSON(t, g)
	f.restart(t)
	replay, err := f.s.warriorAwake(actualContext(awake), f.id, awake, 99740)
	if err != nil || !proto.Equal(replay[0], result[0]) {
		t.Fatal("cold awakening receipt", err)
	}
	p, g = f.profile(t)
	if !proto.Equal(p, beforeP) || string(p5AwakeElementJSON(t, g)) != string(beforeG) {
		t.Fatal("cold source/selection/awakening facts changed")
	}
}

func p5AwakeElementJSON(t *testing.T, game *model.GameState) []byte {
	t.Helper()
	raw, err := json.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestP5AwakeElementSupplyPersonalRefreshKeepsOldDirectory(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	now := LocalPlayNow(f.ctx)
	tx := db.GetIns().WithContext(f.ctx)
	if err := f.s.seedP5AwakeElementSupply(tx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitExchange", "99700000-0000-4000-8000-000000000001"); err != nil {
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
	global := constUtil.P5AwakeElementSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", global).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p5AppendAwakeElementBinding(tx, directory, &bindings, now); err != nil || directory.InputHash != beforeHash || string(directory.Bindings) != beforeRaw {
		t.Fatal("late enabled supply mutated private frozen directory", err)
	}
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "TimeLimitExchange", "99700000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	newKey, err := localPlayExchangeKey(tx, now)
	if err != nil || newKey == key {
		t.Fatal("new personal cycle identity", err)
	}
	_, fresh, err := f.s.p4ExchangeDirectory(tx, newKey, now)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, binding := range fresh {
		if strings.HasPrefix(binding.ChildID, constUtil.P5AwakeElementSupplyPrefix) {
			count++
			if binding.ProductID != "item_337" {
				t.Fatal("unexpected new source product")
			}
		}
	}
	if count != 1 {
		t.Fatal("explicit refresh did not adopt exactly one337 source", count)
	}
	var old db.P4ResourceExchangeDirectory
	if err := tx.First(&old, "id=?", key).Error; err != nil || old.InputHash != beforeHash || string(old.Bindings) != beforeRaw {
		t.Fatal("old directory history changed", err)
	}
}

func TestP5AwakeElementSupplyMonthlyFrozenPriceAndShutdown(t *testing.T) {
	f, key, now := p5AwakeElementFixture(t)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.Products[0].Cost = "coin,600000"
	cfg.Rules, _ = json.Marshal(rules)
	cfg.Enabled = true
	cfg.Frozen, _ = json.Marshal(cfg)
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	// Later live edits cannot replace the published frozen quote.
	if err := db.GetIns().Model(&cfg).Update("rules", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	next := now.AddDate(0, 1, 0)
	if err := f.s.seedP5AwakeElementSupply(db.GetIns(), next); err != nil {
		t.Fatal(err)
	}
	var inherited db.P4CommerceRule
	nextKey := constUtil.P5AwakeElementSupplyPrefix + next.Format("200601")
	if err := db.GetIns().First(&inherited, "id=?", nextKey).Error; err != nil {
		t.Fatal(err)
	}
	_, actual, err := f.s.p4CommerceSnapshot(inherited)
	if err != nil || inherited.Enabled || actual.Products[0].Cost != "coin,600000" {
		t.Fatal("next month must retain frozen terms without auto-enabling publication", err)
	}
	if err = db.GetIns().Model(&inherited).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{next, next.AddDate(0, 1, 0)} {
		if err = f.s.seedP5AwakeElementSupply(db.GetIns(), at); err != nil {
			t.Fatal(err)
		}
		var row db.P4CommerceRule
		if err = db.GetIns().First(&row, "id=?", constUtil.P5AwakeElementSupplyPrefix+at.Format("200601")).Error; err != nil || row.Enabled {
			t.Fatal("restart or next month reenabled operator shutdown", err)
		}
	}
	// Private enabled clones do not turn global publication back on.
	clone := cfg
	clone.ID, clone.Frozen = key+"_lp-00000000000000000000000000", nil
	clone.StartAt, clone.EndAt = next.AddDate(0, 2, 0).UnixMilli(), next.AddDate(0, 3, 0).UnixMilli()
	clone.Enabled = true
	if !strings.Contains(clone.ID, "lp-") || db.GetIns().Create(&clone).Error != nil {
		t.Fatal("private continuity fixture")
	}
	future := next.AddDate(0, 3, 0)
	if err = f.s.seedP5AwakeElementSupply(db.GetIns(), future); err != nil {
		t.Fatal(err)
	}
	var off db.P4CommerceRule
	if err = db.GetIns().First(&off, "id=?", constUtil.P5AwakeElementSupplyPrefix+future.Format("200601")).Error; err != nil || off.Enabled {
		t.Fatal("private clone became global publication", err)
	}
}

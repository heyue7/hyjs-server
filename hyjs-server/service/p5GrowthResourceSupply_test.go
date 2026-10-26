package service

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p5GrowthResourceFixture(t *testing.T) (*p1Fixture, string, time.Time) {
	t.Helper()
	f := p4Setup(t)
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := f.s.seedP5GrowthResourceSupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	key := constUtil.P5GrowthResourceSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	return f, key, now
}

// TestP5GrowthResourceSupplyCatalog closes all 25 raw-input source holes without
// changing existing catalog indices or loosening recipe/skill qualifications.
func TestP5GrowthResourceSupplyCatalog(t *testing.T) {
	f, key, now := p5GrowthResourceFixture(t)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || len(cfg.Frozen) != 0 || cfg.Source != constUtil.P5GrowthResourceSupplySource {
		t.Fatal("unpublished local source must stay closed/unfrozen")
	}
	products, err := f.s.p5GrowthResourceProducts()
	if err != nil || len(products) != 25 {
		t.Fatal("complete original raw catalog", err)
	}
	seen := map[string]bool{}
	for _, product := range products {
		if seen[product.ID] || product.Name == "" || strings.Contains(product.Name, "Item_") || product.TotalLimit != 0 || product.DailyLimit < 1 {
			t.Fatal("source identity/name/renewable daily limits", product)
		}
		seen[product.ID] = true
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now); err == nil {
		t.Fatal("closed source listed")
	}
	if err = db.GetIns().Model(&cfg).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(18)
		p.Coin = proto.Int64(2000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.GetShopItemList()) != 25 {
		t.Fatal("all raw goods visible with original level qualification")
	}
	// Existing mappings are preserved when this child is enabled late.
	directory, bindings, err := f.s.p4ExchangeDirectory(db.GetIns(), p4ExchangeKey(now), now)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]p4ExchangeBinding(nil), bindings...)
	if err = f.s.p5AppendGrowthResourceBindings(db.GetIns(), directory, &bindings, now); err != nil {
		t.Fatal(err)
	}
	for i, binding := range before {
		if bindings[i] != binding {
			t.Fatal("existing exchange index changed")
		}
	}
	count := 0
	for _, binding := range bindings {
		if binding.ChildID == key {
			count++
		}
	}
	if count != 25 {
		t.Fatal("all25 appended exactly once", count)
	}
	oldLen := len(bindings)
	if err = f.s.p5AppendGrowthResourceBindings(db.GetIns(), directory, &bindings, now); err != nil || len(bindings) != oldLen {
		t.Fatal("duplicate append", err)
	}
	// Actual high-grade purchase refuses the low character, without any award.
	high := listing.GetShopItemList()[5]
	request := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: high.Index, Num: proto.Int32(1), CostResources: high.CostResource, GetResources: high.GetResource}
	if _, err = f.s.p4Commerce(f.ctx, f.id, request, 500, now); err == nil {
		t.Fatal("original-source character gate bypass")
	}
	var row db.Profile
	db.GetIns().First(&row, f.id)
	saved := *row.Game
	// Disabled source must retain the first published frozen catalog and old mappings.
	if err = db.GetIns().Model(&cfg).Updates(map[string]any{"enabled": false, "rules": []byte(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.seedP5GrowthResourceSupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&cfg, "id=?", key)
	db.GetIns().First(&row, f.id)
	if cfg.Enabled || len(cfg.Frozen) == 0 || string(cfg.Rules) != "{}" || *row.Game != saved {
		t.Fatal("seed changed operator or rejected purchase changed inventory")
	}
}

// TestP5GrowthResourceSupplyCookingLoop uses actual source purchases and the
// original free recruit, then crafts/eats a permanent dish. The finite level18
// and coin fixture qualifies the transaction; no fish/dish/partner is granted.
func TestP5GrowthResourceSupplyCookingLoop(t *testing.T) {
	f, key, now := p5GrowthResourceFixture(t)
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", key).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(18)
		p.Coin = proto.Int64(2000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	recruited, err := f.s.warriorRecruit(f.ctx, f.id, &pb.CSWarriorRecruit{CostType: proto.Int32(1), NumType: proto.Int32(1), Free: proto.Int32(1)}, 600)
	if err != nil {
		t.Fatal("real free recruit", err)
	}
	wars := recruited[0].(*pb.SCWarriorRecruit).GetGet().GetWars()
	if len(wars) != 1 {
		t.Fatal("real recruit missing partner")
	}
	hero := wars[0]
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	var first *pb.CSBuyActivityShop
	var firstResult proto.Message
	for i, id := range []string{"FishItem_007", "FishItem_013", "MineItem_007"} {
		var item *pb.ActivityShopItem
		for _, v := range listing.GetShopItemList() {
			if v.GetGetResource()[0].GetId() == id {
				item = v
			}
		}
		if item == nil {
			t.Fatal("missing source", id)
		}
		req := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: item.Index, Num: proto.Int32(1), CostResources: item.CostResource, GetResources: item.GetResource}
		bad := proto.Clone(req).(*pb.CSBuyActivityShop)
		bad.CostResources[0].Num = proto.Int32(1)
		if _, err = f.s.p4Commerce(f.ctx, f.id, bad, int64(700+i), now); err == nil {
			t.Fatal("forged price accepted")
		}
		var wg sync.WaitGroup
		results := make(chan proto.Message, 8)
		errs := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, e := f.s.p4Commerce(f.ctx, f.id, req, int64(710+i), now)
				errs <- e
				if e == nil {
					results <- out[0]
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(results)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var expected proto.Message
		for r := range results {
			if expected == nil {
				expected = r
			} else if !proto.Equal(expected, r) {
				t.Fatal("source duplicate reply diverged")
			}
		}
		if i == 0 {
			first = req
			firstResult = expected
		}
	}
	_, g := f.profile(t)
	if p5GrowthSourceOwned(g, "FishItem_007") != 5 || p5GrowthSourceOwned(g, "FishItem_013") != 5 || p5GrowthSourceOwned(g, "MineItem_007") != 5 {
		t.Fatal("actual source inventory")
	}
	warrior, _ := p5Warrior(g, hero.GetGuid())
	before := warrior.GetCooking().GetProp().GetPdef()
	cook := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_027"), Num: proto.Int32(1)}}
	cooked, err := f.s.fishCooking(f.ctx, f.id, cook, 800)
	if err != nil {
		t.Fatal(err)
	}
	if cooked[0].(*pb.SCFishCooking).GetRes().GetSimps()[0].GetId() != "FishItem_027" {
		t.Fatal("original recipe product")
	}
	use := &pb.CSFishUsing{HeroGuid: hero.Guid, CostItem: &pb.BaseResource{Id: proto.String("FishItem_027"), Num: proto.Int32(1)}}
	used, err := f.s.fishUsing(f.ctx, f.id, use, 801)
	if err != nil {
		t.Fatal(err)
	}
	after := used[0].(*pb.SCFishUsing).GetWarrior().GetCooking().GetProp().GetPdef()
	var rule db.CookingRule
	db.GetIns().First(&rule, 1)
	gains := map[string]float64{}
	if json.Unmarshal([]byte(rule.Gains), &gains) != nil || math.Abs(after-before-30) > 1e-9 || after <= before {
		t.Fatal("actual permanent defense gain")
	}
	if _, err = f.s.gemRefine(f.ctx, f.id, &pb.CSMineRefine{MineId: proto.String("MineItem_007"), Num: proto.Int32(5)}, 802); err != nil {
		t.Fatal("actual purchased raw ore refine", err)
	}
	_, g = f.profile(t)
	if p5GrowthSourceOwned(g, "FishItem_007") != 0 || p5GrowthSourceOwned(g, "FishItem_013") != 0 || p5GrowthSourceOwned(g, "FishItem_027") != 0 || p5GrowthSourceOwned(g, "MineItem_007") != 0 {
		t.Fatal("raw and dish consumed once")
	}
	// Advance only the isolated day cursor to prove repeatable daily supply.
	var state db.P4CommerceState
	db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, key)
	var progress p4CommerceProgress
	json.Unmarshal(state.Payload, &progress)
	progress.Day = "20000101"
	if err = saveP4Commerce(db.GetIns(), &state, &progress); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, first, 803, now); err != nil {
		t.Fatal("next-day source", err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.seedP5GrowthResourceSupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(firstResult, p4Trade(t, f, first, 710, now)) {
		t.Fatal("cold source receipt changed")
	}
	_, g = f.profile(t)
	warrior, _ = p5Warrior(g, hero.GetGuid())
	if warrior.GetCooking().GetProp().GetPdef() != after || p5GrowthSourceOwned(g, "FishItem_007") != 5 {
		t.Fatal("cold source/consumption/effect lost")
	}
	t.Log("original free recruit + paid fish/ore; duplicate8 once; recipe027 defense+" + strconv.FormatFloat(after-before, 'f', -1, 64) + "; original refine; day cursor and cold replay")
}

func p5GrowthSourceOwned(g *model.GameState, key string) int64 {
	for _, list := range [][]*pb.SimpleItem{g.Gems, g.Cooking} {
		for _, item := range list {
			if item.GetId() == key {
				return int64(item.GetNum())
			}
		}
	}
	return 0
}

// An already opened personal directory stays immutable. Only a new real tool
// refresh adopts the published source, and returning to an old day keeps quota.
func TestP5GrowthResourceSupplyPersonalDirectoryRefreshAndCalendarQuota(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	now := LocalPlayNow(f.ctx)
	tx := db.GetIns().WithContext(f.ctx)
	if err := f.s.seedP5GrowthResourceSupply(tx, now); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(18)
		p.Coin = proto.Int64(3000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitExchange", "91300000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	// 工厂以实际提交时刻开期，后续查询不能继续使用开期前采样。
	now = LocalPlayNow(f.ctx)
	key, err := localPlayExchangeKey(tx, now)
	if err != nil {
		t.Fatal(err)
	}
	directory, bindings, err := f.s.p4ExchangeDirectory(tx, key, now)
	if err != nil {
		t.Fatal(err)
	}
	oldHash, oldRaw := directory.InputHash, append([]byte(nil), directory.Bindings...)
	global := constUtil.P5GrowthResourceSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", global).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p5AppendGrowthResourceBindings(tx, directory, &bindings, now); err != nil || directory.InputHash != oldHash || string(directory.Bindings) != string(oldRaw) {
		t.Fatal("late publication changed frozen personal directory", err)
	}
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "TimeLimitExchange", "91300000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	newKey, err := localPlayExchangeKey(tx, now)
	if err != nil || newKey == key {
		t.Fatal("new personal directory identity", err)
	}
	_, newer, err := f.s.p4ExchangeDirectory(tx, newKey, now)
	if err != nil {
		t.Fatal(err)
	}
	var selected p4ExchangeBinding
	count := 0
	for _, binding := range newer {
		if strings.HasPrefix(binding.ChildID, constUtil.P5GrowthResourceSupplyPrefix) {
			count++
			if binding.ProductID == "FishItem_007" {
				selected = binding
			}
		}
	}
	if count != 25 || selected.ChildID == "" {
		t.Fatal("new cycle did not adopt all25 original raw materials", count)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(newKey)}, 0, now).(*pb.SCActivityShopInfo)
	var quote *pb.ActivityShopItem
	for _, item := range listing.ShopItemList {
		if item.GetIndex() == selected.Index {
			quote = item
		}
	}
	if quote == nil {
		t.Fatal("aggregate source quote missing")
	}
	request := &pb.CSBuyActivityShop{ActivityId: proto.String(newKey), Index: quote.Index, Num: proto.Int32(20), CostResources: quote.CostResource, GetResources: quote.GetResource}
	p4Trade(t, f, request, 9201, now)
	request = proto.Clone(request).(*pb.CSBuyActivityShop)
	request.Num = proto.Int32(1)
	if _, err := f.s.p4Commerce(f.ctx, id, request, 9202, now); err == nil {
		t.Fatal("daily20 source gate bypass")
	}
	move := func(action, stamp string) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(f.ctx, id, action, stamp); err != nil {
			t.Fatal(err)
		}
		var owner db.Profile
		if err := db.GetIns().First(&owner, id).Error; err != nil {
			t.Fatal(err)
		}
		var err error
		f.ctx, err = f.s.BindLocalPlayContext(f.ctx, &owner)
		if err != nil {
			t.Fatal(err)
		}
	}
	move("nextDay", "91300000-0000-4000-8000-000000000003")
	p4Trade(t, f, request, 9203, LocalPlayNow(f.ctx))
	move("previousDay", "91300000-0000-4000-8000-000000000004")
	if _, err := f.s.p4Commerce(f.ctx, id, request, 9204, LocalPlayNow(f.ctx)); err == nil {
		t.Fatal("calendar rewind erased previously consumed original day quota")
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var old db.P4ResourceExchangeDirectory
		if err := tx.First(&old, "id=?", key).Error; err != nil {
			return err
		}
		if old.InputHash != oldHash || string(old.Bindings) != string(oldRaw) {
			t.Error("retired personal mapping changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, game := f.profile(t)
	if p5GrowthSourceOwned(game, "FishItem_007") != 105 {
		t.Fatal("real source purchase count changed during date rewind")
	}
}

// Published source continuity is independent from OS/activity enable flags.
// Existing months retain operator shutdown; only absent future months inherit.
func TestP5GrowthResourceSupplyMonthContinuityPreservesShutdownAndFrozenPrices(t *testing.T) {
	f, _, _ := p5GrowthResourceFixture(t)
	tx := db.GetIns().WithContext(f.ctx)
	zone := time.FixedZone("UTC+8", 8*3600)
	january := time.Date(2030, 1, 1, 0, 0, 0, 0, zone)
	load := func(at time.Time) db.P4CommerceRule {
		t.Helper()
		if err := f.s.seedP5GrowthResourceSupply(tx, at); err != nil {
			t.Fatal(err)
		}
		var cfg db.P4CommerceRule
		if err := tx.First(&cfg, "id=?", constUtil.P5GrowthResourceSupplyPrefix+at.Format("200601")).Error; err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	prior := load(january)
	if prior.Enabled {
		t.Fatal("unpublished source inherited an unrelated activity flag")
	}
	prior.Enabled = true
	expectedRules := append([]byte(nil), prior.Rules...)
	frozen, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	var edited p4CommerceRules
	if err = json.Unmarshal(prior.Rules, &edited); err != nil {
		t.Fatal(err)
	}
	edited.Products[0].Cost = "coin,90000"
	changed, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Model(&prior).Updates(map[string]any{"enabled": true, "frozen": frozen, "rules": changed}).Error; err != nil {
		t.Fatal(err)
	}
	// A personal clone must not become the global publication predecessor.
	clone := prior
	clone.ID += "_lp-unrelated"
	clone.StartAt = january.AddDate(0, 0, 1).UnixMilli()
	clone.Enabled = false
	clone.Frozen = nil
	if err = tx.Create(&clone).Error; err != nil {
		t.Fatal(err)
	}
	february := load(january.AddDate(0, 1, 0))
	if february.Enabled || string(february.Rules) != string(expectedRules) || len(february.Frozen) != 0 {
		t.Fatal("frozen catalog terms or closed future publication changed")
	}
	if err = tx.Model(&february).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if load(january.AddDate(0, 1, 0)).Enabled {
		t.Fatal("restart restored an operator-stopped month")
	}
	if load(january.AddDate(0, 2, 0)).Enabled {
		t.Fatal("future month bypassed latest operator shutdown")
	}
	december := load(january.AddDate(0, 11, 0))
	if december.Enabled {
		t.Fatal("unpublished successor enabled itself")
	}
	if err = tx.Model(&december).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	year := load(january.AddDate(1, 0, 0))
	if year.Enabled || year.ID != constUtil.P5GrowthResourceSupplyPrefix+"203101" || year.StartAt != january.AddDate(1, 0, 0).UnixMilli() {
		t.Fatal("year boundary must retain identity without auto-enabling publication")
	}
	if err = tx.Model(&year).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if load(january.AddDate(1, 0, 0)).Enabled {
		t.Fatal("new year shutdown changed on restart")
	}
}

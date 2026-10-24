package service

import (
	"encoding/json"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestShopQuantityTiers63Validation(t *testing.T) {
	f, _ := adminModesFixture(t)
	tier := model.AdminShopQuantityTier{Amount: 300, Weight: 1, Cost: model.AdminResourceAmount{ResourceID: "light", Amount: 20}}
	if err := f.s.validateShopQuantityTiers("memory", "item_005", []model.AdminShopQuantityTier{tier}); err != nil {
		t.Fatal(err)
	}
	for _, tiers := range [][]model.AdminShopQuantityTier{{tier, tier}, {{Amount: 1, Weight: 0, Cost: tier.Cost}}, {{Amount: math.MaxInt32 + 1, Weight: 1, Cost: tier.Cost}}, {{Amount: 1, Weight: math.MaxInt64, Cost: tier.Cost}, {Amount: 2, Weight: 1, Cost: tier.Cost}}} {
		if f.s.validateShopQuantityTiers("memory", "item_005", tiers) == nil {
			t.Fatal("invalid tiers accepted", tiers)
		}
	}
	if f.s.validateShopQuantityTiers("memory", "hero_exp_004", []model.AdminShopQuantityTier{tier}) == nil {
		t.Fatal("gold cap")
	}
	if q, e := shopDrawQuantityTier(nil); q != nil || e != nil {
		t.Fatal(q, e)
	}
	if _, e := shopDrawQuantityTier([]model.AdminShopQuantityTier{{Weight: math.MaxInt64}, {Weight: 1}}); e == nil {
		t.Fatal("sum overflow")
	}
}
func TestShopQuantityTiers63FrozenMemory(t *testing.T) {
	tiers := []model.AdminShopQuantityTier{{Amount: 300, Weight: 1, Cost: model.AdminResourceAmount{ResourceID: "light", Amount: 29}}}
	rules := p4CommerceRules{MemoryPolicyVersion: 1}
	for i, id := range []string{"frag_a", "frag_b", "frag_c", "frag_d", "item_005"} {
		p := p4BaseProduct(id, id+",1", "light,1", int32(i+1), 16)
		p.QuantityTiers = tiers
		rules.Products = append(rules.Products, p)
	}
	state := &p4CommerceProgress{Version: 2, Bought: map[string]int32{}}
	if e := memoryRefresh55(rules, state); e != nil {
		t.Fatal(e)
	}
	cfg := db.P4CommerceRule{Kind: "memory"}
	before := memoryProducts55(cfg, rules, state)
	for _, p := range before {
		if p.Rewards != memorySource55(p)+",300" || p.Cost != "light,29" {
			t.Fatal("tier not frozen", p)
		}
	}
	legacyRules := rules
	legacyRules.MemoryPolicyVersion = 0
	legacyRules.Slots = 2
	legacyRefreshed := &p4CommerceProgress{Version: 3}
	if e := memoryRefresh55(legacyRules, legacyRefreshed); e != nil {
		t.Fatal(e)
	}
	for _, p := range memoryProducts55(cfg, legacyRules, legacyRefreshed) {
		if p.Cost != "light,29" {
			t.Fatal("legacy refresh skipped quantity tiers")
		}
	}
	for i := range rules.Products {
		rules.Products[i].Rewards = rules.Products[i].ID + ",5"
		rules.Products[i].Cost = "light,999"
		rules.Products[i].QuantityTiers = nil
	}
	raw, _ := json.Marshal(state)
	var cold p4CommerceProgress
	json.Unmarshal(raw, &cold)
	after := memoryProducts55(cfg, rules, &cold)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("configuration edit changed frozen quote")
	}
	no := false
	rules.Products[0].Enabled = &no
	for _, p := range memoryProducts55(cfg, rules, &cold) {
		if p.SourceProductID == rules.Products[0].ID && p4Visible(cfg, &cold, p) {
			t.Fatal("disabled product revived")
		}
	}
	legacy := &p4CommerceProgress{Slots: []string{"item_005"}, Bought: map[string]int32{"item_005": 1}}
	old := memoryProducts55(cfg, rules, legacy)
	if len(old) != 1 || old[0].Rewards != "item_005,5" || old[0].ID != "item_005" || len(legacy.Offers) != 1 || legacy.Bought["item_005"] != 1 {
		t.Fatal("legacy was rerolled/reset")
	}
}
func TestShopQuantityTiers63FrozenPet(t *testing.T) {
	s := &GameService{}
	tier := []model.AdminShopQuantityTier{{Amount: 300, Weight: 1, Cost: model.AdminResourceAmount{ResourceID: "coinpet", Amount: 37}}}
	cfg := &p9MarketConfig{Slots: 6, Offers: []p9MarketOffer{{ID: "f", Resource: "frag_pet_001", Amount: 1, Currency: "coinpet", Price: 1, Stock: 1, Weight: 1, QuantityTiers: tier}, {ID: "m", Resource: "item_005", Amount: 1, Currency: "coinpet", Price: 1, Stock: 1, Weight: 1, QuantityTiers: tier}}}
	state := &db.P9PetMarketState{WeekKey: "20261019"}
	if e := s.p9MarketSelect(state, cfg); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(state)
	var cold db.P9PetMarketState
	json.Unmarshal(raw, &cold)
	list, e := s.p9MarketListing(&cold, cfg)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range list.ShopItemList {
		if p.Item[0].GetNum() != 300 || p.Price[0].GetNum() != 37 {
			t.Fatal("tier not saved")
		}
	}
	cfg.LiveOffers = map[string]bool{"m": true}
	list, e = s.p9MarketListing(&cold, cfg)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range list.ShopItemList {
		if p.Item[0].GetId() == "frag_pet_001" {
			t.Fatal("disabled source revived")
		}
	}
}

func TestShopQuantityTiers63AdminSettings(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := db.GetIns().AutoMigrate(&db.AdminShopAvailability{}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"memory", "ggo"} {
		page, e := f.s.AdminShop(f.ctx, key)
		if e != nil {
			t.Fatal(e)
		}
		r := page.Settings.Refresh
		if r == nil {
			t.Fatal("missing refresh settings", key)
		}
		r.Enabled = false
		r.FreeCap = 0
		r.FreeRecoverySeconds = 0
		r.PaidCap = 0
		page, e = f.s.AdminSaveShopSettings(f.ctx, actor, key, model.AdminShopSettingsSaveRequest{AdminShopSettings: page.Settings, Version: page.Version})
		if e != nil {
			t.Fatal(key, e)
		}
		if page.Settings.Refresh.Enabled || page.Settings.Refresh.FreeCap != 0 || page.Settings.Refresh.PaidCap != 0 {
			t.Fatal("settings not persisted", key)
		}
	}
	if e := f.s.LoadP9Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	if e := f.s.initializeP9Market(db.GetIns()); e != nil {
		t.Fatal(e)
	}
	var live db.P9PetMarketRule
	db.GetIns().First(&live, "id=?", constUtil.P9MarketRuleID)
	var cfg p9MarketConfig
	json.Unmarshal(live.Input, &cfg)
	pool := []p9MarketOffer{}
	for _, o := range cfg.Offers {
		if o.ID == "item_124" || o.ID == "frag_pet_001" {
			o.QuantityTiers = []model.AdminShopQuantityTier{{Amount: 3, Weight: 1, Cost: model.AdminResourceAmount{ResourceID: "coinpet", Amount: 17}}}
			pool = append(pool, o)
		}
	}
	cfg.Offers = pool
	cfg.Slots = 6
	if e := db.GetIns().Model(&live).Updates(map[string]any{"enabled": true, "input": adminJSON(cfg)}).Error; e != nil {
		t.Fatal(e)
	}
	var before *db.P9PetMarketState
	if e := db.GetIns().Transaction(func(tx *gorm.DB) error { var e error; before, _, e = f.s.p9MarketState(tx, f.id, time.Now()); return e }); e != nil {
		t.Fatal(e)
	}
	frozen := append([]byte(nil), before.Selected...)
	cfg.Offers[0].Price = 999
	cfg.Offers[0].QuantityTiers[0].Amount = 4
	if e := db.GetIns().Model(&live).Update("input", adminJSON(cfg)).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Transaction(func(tx *gorm.DB) error {
		state, accepted, e := f.s.p9MarketState(tx, f.id, time.Now())
		if e != nil {
			return e
		}
		if string(state.Selected) != string(frozen) {
			t.Fatal("query rerolled frozen slots")
		}
		list, e := f.s.p9MarketListing(state, accepted)
		if e != nil {
			return e
		}
		for _, p := range list.ShopItemList {
			if p.Item[0].GetNum() != 3 || p.Price[0].GetNum() != 17 {
				t.Fatal("live edit replaced accepted quote")
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	page, e := f.s.AdminShop(f.ctx, "pet-market")
	if e != nil {
		t.Fatal(e)
	}
	page.Settings.Refresh.PaidCap = 2
	page.Settings.Refresh.Cost = model.AdminResourceAmount{ResourceID: "diamond", Amount: 99}
	page.Settings.Refresh.Enabled = false
	page, e = f.s.AdminSaveShopSettings(f.ctx, actor, "pet-market", model.AdminShopSettingsSaveRequest{AdminShopSettings: page.Settings, Version: page.Version})
	if e != nil {
		t.Fatal(e)
	}
	if page.Settings.Refresh.Enabled || page.Settings.Refresh.PaidCap != 2 || page.Settings.Refresh.Cost.Amount != 99 {
		t.Fatal("pet settings not saved")
	}
}

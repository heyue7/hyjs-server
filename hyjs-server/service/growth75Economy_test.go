package service

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	pb "hyjs-server/model/protocol"

	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

func growth75Fixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f, a := adminModesFixture(t)
	f.s.InstallGrowth74Resources()
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func() error{
		func() error { return f.s.InitializeAdminShopAvailability(f.ctx) },
		func() error { return f.s.InitializeGGO(f.ctx) },
		func() error { return f.s.LoadPaymentTables("../../analysis/tables") },
		func() error { return f.s.InitializePayments(f.ctx) },
		func() error {
			return f.s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json")
		},
		func() error { return f.s.LoadP9Tables("../../analysis/tables") },
		func() error { return f.s.initializeP9Market(db.GetIns()) },
		func() error { return f.s.LoadP13GuildTables("../../analysis/tables") },
	} {
		if err := init(); err != nil {
			t.Fatal(err)
		}
	}
	return f, a
}

func TestGrowth75StableSKUIgnoresQuantityAndRejectsAmbiguity(t *testing.T) {
	s := &GameService{}
	want := growth74Product{Name: "突破水晶", Reward: "item_006", Quantity: 500, Price: 3000}
	items := []model.AdminShopProduct{{ID: "kept", Name: "旧名", Rewards: []model.AdminResourceAmount{{ResourceID: "item_006", Amount: 2000}}, Enabled: true}}
	id, err := s.growth75StableShopID(items, map[string]bool{}, want)
	if err != nil || id != "kept" {
		t.Fatalf("quantity edit changed SKU: %s %v", id, err)
	}
	items = append(items, model.AdminShopProduct{ID: "other", Name: "另一旧名", Rewards: items[0].Rewards, Enabled: true})
	if _, err = s.growth75StableShopID(items, map[string]bool{}, want); err == nil {
		t.Fatal("ambiguous resources silently assigned")
	}
	if s.growth75ResourceSet(growth74Packed("hero_exp_004", 150)) != s.growth75ResourceSet([]model.AdminResourceAmount{{ResourceID: "hero_exp_004", Amount: 1}}) {
		t.Fatal("experience bundle changes resource identity")
	}
}

// Calls every original backend adapter, then repeats to prove prices do not create new SKUs.
func TestGrowth75EconomyAppliesAllRealShopAdaptersAndStableIDs(t *testing.T) {
	f, a := growth75Fixture(t)
	if err := f.s.ApplyGrowth75Economy(f.ctx, a); err != nil {
		t.Fatal("first economic apply", err)
	}
	keys := []string{"ordinary", "popularity", "point", "score", "guild", "ggo", "memory", "pet-market"}
	ids := map[string][]string{}
	for _, key := range keys {
		shop, err := f.s.AdminShop(f.ctx, key)
		if err != nil {
			t.Fatal(key, err)
		}
		for _, p := range shop.Items {
			ids[key] = append(ids[key], p.ID)
			if !p.Enabled {
				continue
			}
			for _, r := range p.Rewards {
				if len(r.ResourceID) > 5 && r.ResourceID[:5] == "frag_" && r.Amount > 10 {
					t.Fatalf("%s fragment burst %+v", key, p)
				}
			}
			if key == "guild" && p.ContributionCost != 100 {
				t.Fatalf("guild contribution %d", p.ContributionCost)
			}
		}
		if key == "point" {
			menus := map[int64]bool{}
			for _, p := range shop.Items {
				if p.Enabled {
					menus[p.Priority/1000] = true
				}
			}
			if len(menus) != 4 {
				t.Fatalf("native trial menus %v", menus)
			}
		}
	}
	if err := f.s.ApplyGrowth75Economy(f.ctx, a); err != nil {
		t.Fatal("repeated economic apply", err)
	}
	for _, key := range keys {
		shop, err := f.s.AdminShop(f.ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		after := []string{}
		for _, p := range shop.Items {
			after = append(after, p.ID)
		}
		if !bytes.Equal(adminJSON(ids[key]), adminJSON(after)) {
			t.Fatalf("%s repeat created/reordered SKU", key)
		}
	}
}

func TestGrowth75PublishedShopTermsAndCountersStayOnSameConsumer(t *testing.T) {
	f, a := growth75Fixture(t)
	if err := db.GetIns().AutoMigrate(&db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}); err != nil {
		t.Fatal(err)
	}
	root, err := f.s.AdminOrdinaryShopSource(db.GetIns(), "point")
	if err != nil {
		t.Fatal(err)
	}
	var old p4CommerceRules
	if err = json.Unmarshal(root.Rules, &old); err != nil {
		t.Fatal(err)
	}
	root.ID = "shop_schedule_growth75_test"
	root.Rules = adminJSON(old)
	root.Frozen = adminJSON(root)
	if err = db.GetIns().Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("shop-window")
	form.Options.ShopKey = "point"
	form.Options.ShopSource = string(adminJSON(root))
	raw := adminJSON(form)
	def := db.AdminActivityDefinition{ID: "growth75-template", Mode: "shop-window", FormJSON: raw, Revision: 1, Enabled: true}
	occ := db.AdminActivityOccurrence{ID: "growth75-occ", ServerID: f.s.OriginalServerID(), TemplateID: def.ID, Mode: "shop-window", Timing: "calendar", ConsumerKey: root.ID, FormJSON: raw, Revision: 1, Enabled: true}
	inst := db.AdminCharacterActivityInstance{PlayerID: f.id, OccurrenceID: occ.ID, ServerID: f.s.OriginalServerID(), ConsumerKey: root.ID, Mode: "shop-window", FormJSON: raw}
	for _, v := range []any{&def, &occ, &inst} {
		if err = db.GetIns().Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	progress := p4CommerceProgress{Day: p1Day(time.Now()), Bought: map[string]int32{"existing-sku": 1}, Total: map[string]int32{"existing-sku": 1}}
	state := db.P4CommerceState{PlayerID: f.id, RuleID: root.ID, Payload: adminJSON(progress)}
	if err = db.GetIns().Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), state.Payload...)
	shop, err := f.s.AdminShop(f.ctx, "point")
	if err != nil {
		t.Fatal(err)
	}
	p := shop.Items[0]
	p.Rewards[0].Amount = 10
	p.Cost.Amount = 15000
	if _, err = f.s.AdminSaveShopProduct(f.ctx, a, "point", p.ID, model.AdminShopProductSaveRequest{AdminShopProduct: p, Version: shop.Version}, false); err != nil {
		t.Fatal(err)
	}
	latest, err := f.s.AdminOrdinaryShopSource(db.GetIns(), "point")
	if err != nil {
		t.Fatal(err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, e := f.s.growth75SyncShopWindows(tx, a, map[string]db.P4CommerceRule{"point": latest})
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	var after db.P4CommerceRule
	if err = db.GetIns().First(&after, "id=?", root.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.ID != root.ID || !bytes.Equal(after.Rules, latest.Rules) {
		t.Fatal("published original consumer not repriced")
	}
	var frozen db.P4CommerceRule
	if json.Unmarshal(after.Frozen, &frozen) != nil || !bytes.Equal(frozen.Rules, latest.Rules) {
		t.Fatal("published frozen catalogue stale")
	}
	var saved db.P4CommerceState
	if err = db.GetIns().First(&saved, "player_id=? AND rule_id=?", f.id, root.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved.Payload, before) {
		t.Fatal("purchase counters reset")
	}
	for _, target := range []any{&def, &occ, &inst} {
		q := db.GetIns()
		switch target.(type) {
		case *db.AdminActivityDefinition:
			q = q.Where("id=?", def.ID)
		case *db.AdminActivityOccurrence:
			q = q.Where("id=?", occ.ID)
		default:
			q = q.Where("player_id=? AND occurrence_id=?", f.id, occ.ID)
		}
		if err = q.First(target).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range [][]byte{def.FormJSON, occ.FormJSON, inst.FormJSON} {
		var got model.AdminActivityForm
		if json.Unmarshal(r, &got) != nil {
			t.Fatal("invalid form")
		}
		var src db.P4CommerceRule
		if json.Unmarshal([]byte(got.Options.ShopSource), &src) != nil || !bytes.Equal(src.Rules, latest.Rules) {
			t.Fatal("source snapshot remains stale")
		}
	}
}

func TestGrowth75QuoteRevisionKeepsSlotAndPaidIdentity(t *testing.T) {
	old := p4Product{ID: "sku", Rewards: "item_006,2000", Cost: "point,50"}
	next := old
	next.Rewards = "item_006,500"
	next.Cost = "point,3000"
	q := ggoQuoteState{Generation: 5, Day: "2026-10-27", PaidUsed: 11, FreeAt: 1234, SlotBought: map[string]int64{"slot0": 1}}
	for i := 0; i < 6; i++ {
		q.Slots = append(q.Slots, ggoQuoteSlot{ID: "slot" + string(rune('0'+i)), SourceProductID: "sku", Product: old})
	}
	if err := growth75RepriceGGO(&q, map[string]p4Product{"sku": next}); err != nil {
		t.Fatal(err)
	}
	if q.Generation != 5 || q.PaidUsed != 11 || q.FreeAt != 1234 || q.SlotBought["slot0"] != 1 || q.Slots[0].ID != "slot0" || q.Slots[0].Product.Cost != "point,3000" {
		t.Fatal("quote revision lost bought/refresh identity")
	}

}

func TestGrowth75ActualPersistedQuotesAndCalendarArchiveKeepQuotaAndReceipts(t *testing.T) {
	f, a := growth75Fixture(t)
	if err := db.GetIns().AutoMigrate(&db.LocalPlayActivityOperation{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyGrowth75Economy(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	// Read the original singleton to avoid assuming seed names.
	var petRule db.P9PetMarketRule
	if err := db.GetIns().First(&petRule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&petRule).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	pet, _, err := f.s.p9MarketState(db.GetIns().WithContext(f.ctx), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var sel []p9MarketOffer
	if json.Unmarshal(pet.Selected, &sel) != nil || len(sel) == 0 {
		t.Fatal("missing native pet quotes")
	}
	pet.Bought = adminJSON(map[string]int32{sel[0].SlotID: 1})
	pet.RefreshUsed = 4
	if err = db.GetIns().Save(pet).Error; err != nil {
		t.Fatal(err)
	}
	if err = calendarQuota75Save(db.GetIns(), f.id, "pet-week", pet.WeekKey, pet); err != nil {
		t.Fatal(err)
	}
	ggo, err := f.s.AdminOrdinaryShopSource(db.GetIns(), "ggo")
	if err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if json.Unmarshal(ggo.Rules, &rules) != nil {
		t.Fatal("ggo rules")
	}
	row, q, err := f.s.ggoLoadQuote(db.GetIns().WithContext(f.ctx), f.id, ggo, rules, true)
	if err != nil {
		t.Fatal(err)
	}
	q.PaidUsed = 9
	q.SlotBought[q.Slots[0].ID] = 1
	if err = ggoSaveQuote(db.GetIns(), row, q); err != nil {
		t.Fatal(err)
	}
	archiveKey := p6Digest([]byte(ggo.ID))[:24] + ":" + q.Day
	if err = calendarQuota75Save(db.GetIns(), f.id, "ggo-day", archiveKey, q); err != nil {
		t.Fatal(err)
	}
	receipt := db.P4Receipt{PlayerID: f.id, Domain: "growth75-proof", Timestamp: 123, Request: []byte("original-request"), Response: []byte("original-response")}
	if err = db.GetIns().Create(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	// Save explicit quantity/price changes through both existing admin adapters.
	petShop, err := f.s.AdminShop(f.ctx, "pet-market")
	if err != nil {
		t.Fatal(err)
	}
	var selected model.AdminShopProduct
	for _, p := range petShop.Items {
		if p.ID == sel[0].ID {
			selected = p
			break
		}
	}
	if selected.ID == "" {
		t.Fatal("pet stable id not found")
	}
	selected.Rewards[0].Amount = 2
	selected.Cost.Amount += 13
	if _, err = f.s.AdminSaveShopProduct(f.ctx, a, "pet-market", selected.ID, model.AdminShopProductSaveRequest{AdminShopProduct: selected, Version: petShop.Version}, false); err != nil {
		t.Fatal(err)
	}
	ggoShop, err := f.s.AdminShop(f.ctx, "ggo")
	if err != nil {
		t.Fatal(err)
	}
	var chosen model.AdminShopProduct
	for _, p := range ggoShop.Items {
		if p.ID == "period:"+q.Slots[0].SourceProductID {
			chosen = p
			break
		}
	}
	if chosen.ID == "" {
		t.Fatal("credit stable source ID not found")
	}
	chosen.Rewards[0].Amount = 1
	chosen.Cost.Amount += 17
	if _, err = f.s.AdminSaveShopProduct(f.ctx, a, "ggo", chosen.ID, model.AdminShopProductSaveRequest{AdminShopProduct: chosen, Version: ggoShop.Version}, false); err != nil {
		t.Fatal(err)
	}
	ggo, err = f.s.AdminOrdinaryShopSource(db.GetIns(), "ggo")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		return f.s.growth75SyncShopQuotes(tx, a, map[string]db.P4CommerceRule{ggo.ID: ggo}, map[string]db.P4CommerceRule{"ggo": ggo})
	}); err != nil {
		t.Fatal(err)
	}
	var afterPet db.P9PetMarketState
	if err = db.GetIns().First(&afterPet, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if afterPet.RefreshUsed != 4 || afterPet.Generation != pet.Generation || !bytes.Equal(afterPet.Bought, pet.Bought) {
		t.Fatal("pet revision reset quota")
	}
	var gotSel []p9MarketOffer
	if json.Unmarshal(afterPet.Selected, &gotSel) != nil || gotSel[0].Amount != 2 || gotSel[0].Price != int32(selected.Cost.Amount) || gotSel[0].SlotID != sel[0].SlotID {
		t.Fatal("persisted native pet quotation stale")
	}
	var afterQ db.P4CommerceState
	if err = db.GetIns().First(&afterQ, "player_id=? AND rule_id=?", f.id, row.RuleID).Error; err != nil {
		t.Fatal(err)
	}
	var got ggoQuoteState
	if json.Unmarshal(afterQ.Payload, &got) != nil || got.Generation != q.Generation || got.PaidUsed != 9 || got.SlotBought[q.Slots[0].ID] != 1 || got.Slots[0].Product.Cost != shopResourceSpec(chosen.Cost) {
		t.Fatal("persisted credit quotation stale or quota reset")
	}
	var archived db.P9PetMarketState
	ok, err := calendarQuota75Load(db.GetIns(), f.id, "pet-week", pet.WeekKey, &archived)
	if err != nil || !ok || archived.SelectedHash != afterPet.SelectedHash || !bytes.Equal(archived.Bought, pet.Bought) {
		t.Fatal("pet archived prices stale", err)
	}
	var oldQ ggoQuoteState
	ok, err = calendarQuota75Load(db.GetIns(), f.id, "ggo-day", archiveKey, &oldQ)
	if err != nil || !ok || oldQ.Slots[0].Product.Cost != got.Slots[0].Product.Cost || oldQ.PaidUsed != 9 {
		t.Fatal("credit archived prices stale", err)
	}
	var afterReceipt db.P4Receipt
	if err = db.GetIns().First(&afterReceipt, "player_id=? AND domain=? AND timestamp=?", f.id, receipt.Domain, receipt.Timestamp).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterReceipt.Request, receipt.Request) || !bytes.Equal(afterReceipt.Response, receipt.Response) {
		t.Fatal("immutable receipt mutated")
	}
}

func TestGrowth75MemoryAndPetNativeOpenConfigurationRetainsIdentity(t *testing.T) {
	f, a := growth75Fixture(t)
	if err := f.s.InitializeP9Pets(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(80); return nil }); err != nil {
		t.Fatal(err)
	}
	beforeMemory, err := f.s.AdminShop(f.ctx, "memory")
	if err != nil {
		t.Fatal(err)
	}
	// This test specifically starts from a closed existing source. Shared P4
	// fixtures may explicitly enable it for their own protocol scenarios.
	if err = db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", beforeMemory.Settings.PeriodID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, time.Now()); err == nil {
		t.Fatal("default memory source unexpectedly open")
	}
	if _, err = f.s.p9PetShopList(f.ctx, f.id); err == nil {
		t.Fatal("default pet market unexpectedly open")
	}
	if err = f.s.ApplyGrowth75Economy(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, time.Now())
	if err != nil || len(messages) == 0 {
		t.Fatal("native altar source still closed", err)
	}
	afterMemory, err := f.s.AdminShop(f.ctx, "memory")
	if err != nil || afterMemory.Settings.PeriodID != beforeMemory.Settings.PeriodID {
		t.Fatal("opening replaced stable memory period", err)
	}
	listed, err := f.s.p9PetShopList(f.ctx, f.id)
	if err != nil || len(listed) == 0 || len(listed[0].(*pb.SCPetShopProductList).GetShopItemList()) != 6 {
		t.Fatal("native pet shop not six real quotes", err)
	}
	var before db.P9PetMarketState
	if err = db.GetIns().First(&before, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyGrowth75Economy(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	var after db.P9PetMarketState
	if err = db.GetIns().First(&after, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if before.Generation != after.Generation || before.WeekKey != after.WeekKey || before.RefreshUsed != after.RefreshUsed || !bytes.Equal(before.Bought, after.Bought) {
		t.Fatal("reapply opening reset pet quotas")
	}
}

func TestGrowth75MemoryPreservesActiveIdentityAheadOfClosedFutureSeed(t *testing.T) {
	f, a := growth75Fixture(t)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(80); return nil }); err != nil {
		t.Fatal(err)
	}
	shop, err := f.s.AdminShop(f.ctx, "memory")
	if err != nil {
		t.Fatal(err)
	}
	settings := shop.Settings
	settings.Enabled = true
	settings.Unlimited = true
	if _, err = f.s.AdminSaveShopSettings(f.ctx, a, "memory", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	var saved db.P4CommerceState
	if err = db.GetIns().First(&saved, "player_id=? AND rule_id=?", f.id, settings.PeriodID).Error; err != nil {
		t.Fatal(err)
	}
	var progress p4CommerceProgress
	if json.Unmarshal(saved.Payload, &progress) != nil {
		t.Fatal("missing original memory progress")
	}
	progress.FreeLeft = 2
	progress.ResourceUsed = 3
	saved.Payload = adminJSON(progress)
	if err = db.GetIns().Save(&saved).Error; err != nil {
		t.Fatal(err)
	}
	var future db.P4CommerceRule
	if err = db.GetIns().First(&future, "id=?", settings.PeriodID).Error; err != nil {
		t.Fatal(err)
	}
	future.ID = "p4:memory:closed-future75"
	future.StartAt = time.Now().AddDate(0, 1, 0).UnixMilli()
	future.EndAt = time.Now().AddDate(0, 2, 0).UnixMilli()
	future.ClaimEndAt = future.EndAt
	future.Enabled = false
	future.Frozen = nil
	if err = db.GetIns().Create(&future).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyGrowth75Economy(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	source, err := f.s.AdminOrdinaryShopSource(db.GetIns(), "memory")
	if err != nil || source.ID != settings.PeriodID {
		t.Fatal("future disabled seed replaced operating quota identity", source.ID, err)
	}
	var after db.P4CommerceState
	if err = db.GetIns().First(&after, "player_id=? AND rule_id=?", f.id, settings.PeriodID).Error; err != nil {
		t.Fatal(err)
	}
	var got p4CommerceProgress
	if json.Unmarshal(after.Payload, &got) != nil || got.Version != progress.Version || got.FreeLeft != 2 || got.ResourceUsed != 3 {
		t.Fatal("opening restored refresh uses")
	}
	if n := p1Count(t, &db.P4CommerceState{}, "player_id=? AND rule_id=?", f.id, future.ID); n != 0 {
		t.Fatal("new quota shelf created")
	}
}

// Real GORM CAS must distinguish SQL NULL from an empty BLOB. All three shapes
// occur in published consumers, and a frozen snapshot is one flat document.
func TestGrowth75NativeShopCASNullEmptyAndFrozenIsStable(t *testing.T) {
	f, a := growth75Fixture(t)
	if err := f.s.ApplyGrowth75Economy(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	source, err := f.s.AdminOrdinaryShopSource(db.GetIns(), "guild")
	if err != nil {
		t.Fatal(err)
	}
	for _, shape := range []string{"null", "empty", "frozen"} {
		t.Run(shape, func(t *testing.T) {
			before := source
			before.ID = "shop_schedule_growth75_cas_" + shape
			before.Source = "accepted old operating terms"
			before.Frozen = nil
			if shape == "empty" {
				before.Frozen = []byte{}
			}
			if shape == "frozen" {
				before.Frozen = adminJSON(before)
			}
			if err := db.GetIns().Create(&before).Error; err != nil {
				t.Fatal(err)
			}
			state := db.P4CommerceState{PlayerID: f.id, RuleID: before.ID, Payload: []byte(`{"bought":{"kept":1}}`)}
			if err := db.GetIns().Create(&state).Error; err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.growth75SyncNativeShop(tx, a, before.ID, source) }); err != nil {
					t.Fatal(shape, attempt, err)
				}
				var after db.P4CommerceRule
				if err := db.GetIns().First(&after, "id=?", before.ID).Error; err != nil {
					t.Fatal(err)
				}
				if after.ID != before.ID || after.StartAt != before.StartAt || after.EndAt != before.EndAt || after.Enabled != before.Enabled || !bytes.Equal(after.Rules, source.Rules) {
					t.Fatal("consumer identity or operating terms lost")
				}
				if shape == "frozen" {
					var frozen db.P4CommerceRule
					if json.Unmarshal(after.Frozen, &frozen) != nil || len(frozen.Frozen) != 0 || !bytes.Equal(frozen.Rules, source.Rules) {
						t.Fatal("recursive or stale frozen document")
					}
				} else if len(after.Frozen) != 0 {
					t.Fatal("unfrozen consumer became frozen")
				}
				var saved db.P4CommerceState
				if err := db.GetIns().First(&saved, "player_id=? AND rule_id=?", f.id, before.ID).Error; err != nil || !bytes.Equal(saved.Payload, state.Payload) {
					t.Fatal("accepted bought state changed", err)
				}
				before = after
			}
		})
	}
}

package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

// A finite point-shop fixture checks real native purchases and preserved history
// across operator edits, rejected stale prices and process/database reopen.
func TestLiveStandardShopMaterialsQuotesQuotaAndCold(t *testing.T) {
	f, actor := adminModesFixture(t)
	if e := f.s.InitializeAdminShopAvailability(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"MineSkill", "FishSkill"} {
		rows, e := adminCatalogueReadTSV("../../analysis/tables/Table" + name + ".tsv")
		if e != nil {
			t.Fatal(e)
		}
		f.s.Tables[name] = rows
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Point = proto.Int32(1000)
		g.Items = []*pb.SimpleItem{{Id: proto.String("frag_hero_001"), Num: proto.Int32(20)}}
		g.Gems = []*pb.SimpleItem{{Id: proto.String("MineItem_007"), Num: proto.Int32(20)}}
		g.Cooking = []*pb.SimpleItem{{Id: proto.String("FishItem_007"), Num: proto.Int32(20)}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	shop, e := f.s.AdminShop(f.ctx, "point")
	if e != nil {
		t.Fatal(e)
	}
	settings := shop.Settings
	settings.Enabled = true
	settings.Unlimited = true
	shop, e = f.s.AdminSaveShopSettings(f.ctx, actor, "point", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version})
	if e != nil {
		t.Fatal(e)
	}
	item := shop.Items[0]
	item.Cost = model.AdminResourceAmount{ResourceID: "frag_hero_001", Amount: 2}
	item.Rewards = []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 1}}
	item.DailyLimit = 3
	item.TotalLimit = 3
	item.Stock = 3
	item.MinLevel = 1
	item.MinVip = 0
	item.DiscountBps = 10000
	save := func() {
		t.Helper()
		current, x := f.s.AdminShop(f.ctx, "point")
		if x != nil {
			t.Fatal(x)
		}
		shop, x = f.s.AdminSaveShopProduct(f.ctx, actor, "point", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: current.Version}, false)
		if x != nil {
			t.Fatal(x)
		}
	}
	save()
	list := func() *pb.ShopItem {
		t.Helper()
		out, x := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(5)}, 0, time.Now())
		if x != nil {
			t.Fatal(x)
		}
		for _, p := range out[0].(*pb.SCShopProductList).GetShopItemList() {
			_, stable, ok := adminLiveShopParseSKU(p.GetId())
			if ok && stable == strings.TrimPrefix(item.ID, "period:") {
				return p
			}
		}
		t.Fatal("configured product absent")
		return nil
	}
	buy := func(p *pb.ShopItem, stamp int64) ([]proto.Message, error) {
		return f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: p.Id, BuyCount: proto.Int32(1)}, stamp, time.Now())
	}
	old := list()
	if old.GetCanBuy() != 0 || old.GetCondition().GetValue() != 100 {
		t.Fatal("low-star trial eligibility not projected", old)
	}
	if _, e = buy(old, 909999); e == nil {
		t.Fatal("low-star material purchase accepted")
	}
	p, g := f.profile(t)
	if p4ResourceBalance(p, g, "MineItem_007") != 20 || simpleOwned(p, g, "item_005") != 0 {
		t.Fatal("rejected low-star buy changed inventory")
	}
	// Explicit eligibility fixture; this does not claim natural 100-star progression.
	if e = db.GetIns().Create(&db.P7RushState{PlayerID: f.id, RecordStars: 100}).Error; e != nil {
		t.Fatal(e)
	}
	firstStable := strings.TrimPrefix(item.ID, "period:")
	item.Cost = model.AdminResourceAmount{ResourceID: "MineItem_007", Amount: 3}
	save()
	if _, e = buy(old, 910001); e == nil {
		t.Fatal("old quote accepted")
	}
	fresh := list()
	first, e := buy(fresh, 910001)
	if e != nil {
		t.Fatal(e)
	}
	if len(first) != 4 {
		t.Fatal("current inventory reconciliation missing", len(first))
	}
	p, g = f.profile(t)
	if p4ResourceBalance(p, g, "MineItem_007") != 17 || simpleOwned(p, g, "item_005") != 1 || simpleOwned(p, g, "frag_hero_001") != 20 {
		t.Fatal("real material charge mismatch")
	}
	item.Cost = model.AdminResourceAmount{ResourceID: "FishItem_007", Amount: 4}
	save()
	if purchased := list(); purchased.GetCanBuy() != 2 || purchased.GetDayLeftTimes() != 0 {
		t.Fatal("edit reopened lifetime quota", purchased)
	}
	if _, e = buy(list(), 910002); e == nil {
		t.Fatal("edited trial SKU purchased twice")
	}
	var stock db.P4CommerceStock
	if e = db.GetIns().First(&stock, "rule_id=? AND product_id=?", shop.Settings.PeriodID, firstStable).Error; e != nil || stock.Used != 1 {
		t.Fatal("first product stock changed", stock, e)
	}
	// A distinct stable product keeps both material paths covered under one-time trial limits.
	item = shop.Items[1]
	item.Cost = model.AdminResourceAmount{ResourceID: "FishItem_007", Amount: 4}
	item.Rewards = []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 1}}
	item.DailyLimit = 3
	item.TotalLimit = 3
	item.Stock = 3
	item.MinLevel = 1
	item.MinVip = 0
	item.DiscountBps = 10000
	stars := int64(100)
	item.MinTrialStars = &stars
	secondStable := strings.TrimPrefix(item.ID, "period:")
	save()
	next := list()
	if next.GetDayLeftTimes() != 1 || next.GetCanBuy() != 1 {
		t.Fatal("new trial product quota", next)
	}
	if _, e = buy(next, 910003); e != nil {
		t.Fatal(e)
	}
	item.DailyLimit = 1
	item.TotalLimit = 1
	item.Stock = 1
	save()
	if list().GetCanBuy() != 2 {
		t.Fatal("lowered limits reopened inventory")
	}
	f.restart(t)
	replay, e := buy(fresh, 910001)
	if e != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("cold committed receipt", e)
	}
	p, g = f.profile(t)
	if p4ResourceBalance(p, g, "MineItem_007") != 17 || p4ResourceBalance(p, g, "FishItem_007") != 16 || simpleOwned(p, g, "item_005") != 2 {
		t.Fatal("cold replay changed material balance")
	}
	if len(replay) != 4 || replay[2].(*pb.SCGemAndCookingList).GetCookingList()[0].GetNum() != 16 {
		t.Fatal("replay did not reconcile current inventory")
	}
	var row db.P4CommerceState
	if e = db.GetIns().First(&row, "player_id=? AND rule_id=?", f.id, shop.Settings.PeriodID).Error; e != nil {
		t.Fatal(e)
	}
	var progress p4CommerceProgress
	if e = json.Unmarshal(row.Payload, &progress); e != nil || progress.Total[firstStable] != 1 || progress.Total[secondStable] != 1 {
		t.Fatal("history changed", e)
	}
	// Preferred resources rank first without restricting search or pagination.
	data := f.s.adminActivityCostCatalogue()
	request := model.AdminItemsRequest{Page: 1, PageSize: 1, PreferredIDs: []string{"FishItem_007"}}
	ordered := (&GameService{AdminCatalogueData: data}).AdminCatalogueFiltered(request)
	if len(ordered.Items) != 1 || ordered.Items[0].ID != "FishItem_007" || ordered.Total < 100 {
		t.Fatal("recommended material restricted catalogue")
	}
	request.Q = "金币"
	searched := (&GameService{AdminCatalogueData: data}).AdminCatalogueFiltered(request)
	if searched.Total == 0 || searched.Items[0].ID == "FishItem_007" {
		t.Fatal("recommendation bypassed search")
	}
	if _, e = f.s.p4ParseCost("unionFightToken,1"); e == nil {
		t.Fatal("battle-scoped token accepted outside guild battle")
	}
}

// Guild costs may use the same resource twice. Sum it before deduction, keep
// original contribution ranking untouched, and reconcile replayed balances.
func TestLiveGuildShopFlexibleCostsAndContribution(t *testing.T) {
	f, actor := adminModesFixture(t)
	if e := f.s.LoadP13GuildTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeP13Guild(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeAdminShopAvailability(f.ctx); e != nil {
		t.Fatal(e)
	}
	for _, v := range []any{&db.P13Guild{ID: "live-guild", Name: "隔离商店", Level: 1}, &db.P13GuildMember{PlayerID: f.id, GuildID: "live-guild", Contribution: 8}} {
		if e := db.GetIns().Create(v).Error; e != nil {
			t.Fatal(e)
		}
	}
	if e := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Updates(map[string]any{"management_enabled": true, "economy_enabled": true}).Error; e != nil {
		t.Fatal(e)
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(40)
		p.Contribution = proto.Int32(8)
		p.Coinpet = proto.Int64(10)
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	shop, e := f.s.AdminShop(f.ctx, "guild")
	if e != nil {
		t.Fatal(e)
	}
	settings := shop.Settings
	settings.Enabled = true
	settings.Unlimited = true
	shop, e = f.s.AdminSaveShopSettings(f.ctx, actor, "guild", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version})
	if e != nil {
		t.Fatal(e)
	}
	item := shop.Items[0]
	item.Cost = model.AdminResourceAmount{ResourceID: "contribution", Amount: 5}
	item.ContributionCost = 4
	item.DailyLimit = 3
	item.Stock = -1
	item.TotalLimit = 0
	save := func() {
		t.Helper()
		current, x := f.s.AdminShop(f.ctx, "guild")
		if x != nil {
			t.Fatal(x)
		}
		shop, x = f.s.AdminSaveShopProduct(f.ctx, actor, "guild", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: current.Version}, false)
		if x != nil {
			t.Fatal(x)
		}
	}
	save()
	list := func() *pb.ShopItem {
		t.Helper()
		v, x := f.s.p4GuildShop(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(8)}, 0, time.Now())
		if x != nil {
			t.Fatal(x)
		}
		return v[0].(*pb.SCShopProductList).GetShopItemList()[0]
	}
	buy := func(p *pb.ShopItem, stamp int64) ([]proto.Message, error) {
		return f.s.p4GuildShop(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(8), ShopItemId: p.Id, BuyCount: proto.Int32(1)}, stamp, time.Now())
	}
	if _, e = buy(list(), 99101); e == nil {
		t.Fatal("two contribution costs exceeded actual balance")
	}
	p, _ := f.profile(t)
	if p.GetContribution() != 8 {
		t.Fatal("rejected purchase changed balance")
	}
	item.ContributionCost = 2
	save()
	first, e := buy(list(), 99101)
	if e != nil {
		t.Fatal(e)
	}
	p, _ = f.profile(t)
	if p.GetContribution() != 1 || len(first) != 4 {
		t.Fatal("actual contribution debit/current snapshots")
	}
	item.Cost = model.AdminResourceAmount{ResourceID: "coinpet", Amount: 3}
	item.ContributionCost = 0
	save()
	second, e := buy(list(), 99102)
	if e != nil {
		t.Fatal("zero extra contribution/generic currency", e)
	}
	if len(second) != 4 {
		t.Fatal("reconciliation missing")
	}
	f.restart(t)
	p, _ = f.profile(t)
	if p.GetCoinpet() != 7 || p.GetContribution() != 1 {
		t.Fatal("generic guild debit cold")
	}
	var member db.P13GuildMember
	if e = db.GetIns().First(&member, f.id).Error; e != nil || member.Contribution != 8 {
		t.Fatal("cumulative guild rank changed", e)
	}
}

func TestLiveLegacyOrdinaryMaterialsQuoteAndQuota(t *testing.T) {
	f, actor := adminModesFixture(t)
	if e := f.s.InitializeAdminShopAvailability(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		g.Items = []*pb.SimpleItem{{Id: proto.String("frag_hero_001"), Num: proto.Int32(20)}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	shop, e := f.s.AdminShop(f.ctx, "ordinary")
	if e != nil {
		t.Fatal(e)
	}
	var item model.AdminShopProduct
	for _, v := range shop.Items {
		if strings.HasPrefix(v.ID, "legacy:") {
			item = v
			break
		}
	}
	if item.ID == "" {
		t.Fatal("legacy catalogue missing")
	}
	item.Enabled = true
	item.MinLevel = 1
	item.MinVip = 0
	item.DailyLimit = 3
	item.Rewards = []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 1}}
	item.Cost = model.AdminResourceAmount{ResourceID: "frag_hero_001", Amount: 2}
	save := func() {
		t.Helper()
		current, x := f.s.AdminShop(f.ctx, "ordinary")
		if x != nil {
			t.Fatal(x)
		}
		_, x = f.s.AdminSaveShopProduct(f.ctx, actor, "ordinary", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: current.Version}, false)
		if x != nil {
			t.Fatal(x)
		}
	}
	save()
	list := func() *pb.ShopItem {
		t.Helper()
		v, x := f.s.shopProducts(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(1)})
		if x != nil {
			t.Fatal(x)
		}
		for _, p := range v[0].(*pb.SCShopProductList).GetShopItemList() {
			_, id, q := adminLiveShopParseSKU(p.GetId())
			if q && id == strings.TrimPrefix(item.ID, "legacy:") {
				return p
			}
		}
		t.Fatal("live ordinary entry absent")
		return nil
	}
	old := list()
	item.Cost.Amount = 3
	save()
	buy := func(v *pb.ShopItem, stamp int64) ([]proto.Message, error) {
		return f.s.shopBuy(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: v.Id, BuyCount: proto.Int32(1)}, stamp)
	}
	if _, e = buy(old, 99301); e == nil {
		t.Fatal("old ordinary price charged")
	}
	fresh := list()
	first, e := buy(fresh, 99301)
	if e != nil || len(first) != 4 {
		t.Fatal("native ordinary material buy", e)
	}
	item.DailyLimit = 1
	save()
	if list().GetCanBuy() != 0 {
		t.Fatal("ordinary edit reset quota")
	}
	f.restart(t)
	v, e := buy(fresh, 99301)
	if e != nil || !proto.Equal(v[0], first[0]) {
		t.Fatal("ordinary cold receipt", e)
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "frag_hero_001") != 17 || simpleOwned(p, g, "item_005") != 1 {
		t.Fatal("ordinary cold balance")
	}
}

func TestLiveRankClosedSeedCanEditMaterialsWithoutOpening(t *testing.T) {
	f, actor := adminModesFixture(t)
	shop, e := f.s.AdminShop(f.ctx, "popularity-rank")
	if e != nil || len(shop.Items) != 1 {
		t.Fatal("original rank seed", e)
	}
	id := shop.Settings.PeriodID
	item := shop.Items[0]
	item.Cost = model.AdminResourceAmount{ResourceID: "frag_hero_001", Amount: 20}
	result, e := f.s.AdminSaveShopProduct(f.ctx, actor, "popularity-rank", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: shop.Version}, false)
	if e != nil || result.Settings.Enabled || result.Settings.PeriodID != id || result.Items[0].Cost.ResourceID != "frag_hero_001" {
		t.Fatal("closed seed material edit", e)
	}
}

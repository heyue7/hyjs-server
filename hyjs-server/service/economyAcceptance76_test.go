package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Finite SQLite inventory exercises published 75 prices through native consumers.
// It is separate from QA1001 and does not represent naturally earned income.
func economy76Fixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f, actor := growth75Fixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, initialize := range []func(context.Context) error{f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities, f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	actor.Role = constUtil.AdminRoleAdmin
	for _, initialize := range []func(context.Context) error{f.s.InitializeP9Pets, f.s.InitializeP9PetNext, f.s.InitializeP9PetRecycle} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.ApplyGrowth75Economy(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Vip = proto.Int32(100), proto.Int32(12)
		p.Point, p.Popularity, p.Score = proto.Int32(2000000), proto.Int32(10000), proto.Int32(10000)
		p.Light = proto.Int32(1000)
		p.Diamond, p.Coinpet = proto.Int64(10000), proto.Int64(50000)
		_, err := f.s.GrantFixedReward(p, g, "item_9710,10000")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f, actor
}

func TestEconomy76ActualPopularityRedFragmentsDailyOnce(t *testing.T) {
	f, actor := economy76Fixture(t)
	now := time.Now()
	out, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(4)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	var sku *pb.ShopItem
	for _, item := range out[0].(*pb.SCShopProductList).GetShopItemList() {
		for _, reward := range item.GetItem() {
			if reward.GetId() == "item_120" {
				sku = item
			}
			if strings.HasPrefix(reward.GetId(), "frag_hero_") {
				t.Fatal("orange hero fragments remain", item)
			}
		}
	}
	if sku == nil || sku.GetItem()[0].GetNum() != 10 || sku.GetPrice()[0].GetNum() != 1500 || sku.GetDayLeftTimes() != 1 {
		t.Fatal("actual red selector terms", sku)
	}
	request := &pb.CSShopBuy{ShopType: proto.Int32(4), ShopItemId: sku.Id, BuyCount: proto.Int32(1)}
	before, g0 := f.profile(t)
	paid, err := f.s.p4Commerce(f.ctx, f.id, request, 760001, now)
	if err != nil {
		t.Fatal(err)
	}
	after, g1 := f.profile(t)
	if before.GetPopularity()-after.GetPopularity() != 1500 || simpleOwned(after, g1, "item_120")-simpleOwned(before, g0, "item_120") != 10 {
		t.Fatal("actual price/reward not applied")
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, request, 760002, now); err == nil {
		t.Fatal("second same-day purchase accepted")
	}
	denied, gd := f.profile(t)
	if !proto.Equal(after, denied) || !bytes.Equal(adminJSON(g1), adminJSON(gd)) {
		t.Fatal("denial mutated profile")
	}
	replay, err := f.s.p4Commerce(f.ctx, f.id, request, 760001, now)
	if err != nil || !proto.Equal(paid[0], replay[0]) {
		t.Fatal("original receipt replay", err)
	}
	reuseClockDay(t, f, actor)
	bound := serverClockBindTest(t, f, f.id)
	if _, err = f.s.p4Commerce(bound, f.id, request, 760003, LocalPlayNow(bound)); err != nil {
		t.Fatal("next day did not restore daily quota", err)
	}
	end, _ := f.profile(t)
	if end.GetPopularity() != before.GetPopularity()-3000 {
		t.Fatal("next-day currency", end.GetPopularity())
	}
}

func TestEconomy76ActualPetCatalogueCurrencyAndRandomRefresh(t *testing.T) {
	f, _ := economy76Fixture(t)
	first, err := f.s.p9PetShopList(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	list := first[0].(*pb.SCPetShopProductList)
	if len(list.GetShopItemList()) != 6 || list.GetRefreshTimes() != 10 {
		t.Fatal("actual six-slot weekly catalogue", list)
	}
	var state0 db.P9PetMarketState
	if err = db.GetIns().First(&state0, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range list.GetShopItemList() {
		if item.GetPrice()[0].GetId() != "coinpet" || item.GetPrice()[0].GetNum() <= 0 {
			t.Fatal("pet currency/price", item)
		}
		for _, reward := range item.GetItem() {
			if strings.HasPrefix(reward.GetId(), "frag_") && reward.GetNum() > 10 {
				t.Fatal("fragment burst", item)
			}
		}
	}
	quoted := list.GetShopItemList()[0]
	request := &pb.CSPetShopBuy{ShopItemId: quoted.Id, BuyCount: proto.Int32(1)}
	before, _ := f.profile(t)
	paid, err := f.s.p9PetShopBuy(f.ctx, f.id, request, 761001)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if before.GetCoinpet()-after.GetCoinpet() != int64(quoted.GetPrice()[0].GetNum()) || before.GetCoin() != after.GetCoin() || before.GetDiamond() != after.GetDiamond() {
		t.Fatal("wrong currency deducted")
	}
	replay, err := f.s.p9PetShopBuy(f.ctx, f.id, request, 761001)
	if err != nil || !proto.Equal(paid[0], replay[0]) {
		t.Fatal("pet buy replay", err)
	}
	refresh := &pb.CSRefreshPetShop{}
	refreshed, err := f.s.p9PetShopRefresh(f.ctx, f.id, refresh, 761002)
	if err != nil {
		t.Fatal(err)
	}
	end, _ := f.profile(t)
	if after.GetDiamond()-end.GetDiamond() != 150 || after.GetCoinpet() != end.GetCoinpet() {
		t.Fatal("refresh did not charge original 150 diamonds")
	}
	var state1 db.P9PetMarketState
	if err = db.GetIns().First(&state1, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if state1.Generation != state0.Generation+1 || state1.RefreshUsed != 1 || bytes.Equal(state0.Selected, state1.Selected) {
		t.Fatal("refresh did not create new frozen slot epoch")
	}
	if _, err = f.s.p9PetShopBuy(f.ctx, f.id, request, 761003); err == nil {
		t.Fatal("stale old slot accepted")
	}
	again, err := f.s.p9PetShopRefresh(f.ctx, f.id, refresh, 761002)
	if err != nil || !proto.Equal(refreshed[0], again[0]) {
		t.Fatal("refresh replay", err)
	}
	last, _ := f.profile(t)
	if last.GetDiamond() != end.GetDiamond() || last.GetCoinpet() != end.GetCoinpet() {
		t.Fatal("replay charged twice")
	}
}

func TestEconomy76ActualCreditFifteenRefreshCapAndFrozenQuotes(t *testing.T) {
	f, _ := economy76Fixture(t)
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	page, err := f.s.GGOShopCatalogue(ctx, f.id, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 6 || page.Refresh == nil || page.Refresh.PaidLeft != 15 {
		t.Fatal("actual credit refresh policy", page)
	}
	initial := page.Balance
	for i := 0; i < 15; i++ {
		req := model.GGOShopRefreshRequest{PlayerID: f.id, Version: page.Version, RequestID: uuid.NewString()}
		out, e := f.s.GGOShopRefresh(ctx, req, owner.Session)
		if e != nil || out.CostAmount != 10 || out.CostID != "item_9710" {
			t.Fatal("actual paid refresh", i, out, e)
		}
		again, e := f.s.GGOShopRefresh(ctx, req, owner.Session)
		if e != nil || !bytes.Equal(adminJSON(out), adminJSON(again)) {
			t.Fatal("credit refresh replay", i, e)
		}
		page = out.Catalogue
	}
	if page.Balance != initial-150 || page.Refresh.PaidLeft != 0 {
		t.Fatal("credit cap/cost", page)
	}
	before, g0 := f.profile(t)
	if _, err = f.s.GGOShopRefresh(ctx, model.GGOShopRefreshRequest{PlayerID: f.id, Version: page.Version, RequestID: uuid.NewString()}, owner.Session); err == nil {
		t.Fatal("sixteenth paid refresh accepted")
	}
	after, g1 := f.profile(t)
	if !proto.Equal(before, after) || !bytes.Equal(adminJSON(g0), adminJSON(g1)) {
		t.Fatal("over-cap rejection changed assets")
	}
	var rule db.P4CommerceRule
	if err = db.GetIns().First(&rule, "id=?", constUtil.GGOSeedRuleID).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err = json.Unmarshal(rule.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	var weight int64
	for _, p := range rules.Products {
		if p.isEnabled() {
			weight += p.Weight
		}
	}
	if weight != 10000 {
		t.Fatal("75 credit weights do not sum to10000", weight)
	}
	t.Logf("credit actual 15 paid refreshes: %d -> %d credits; generation=%d", initial, page.Balance, page.Refresh.Generation)
	for _, p := range rules.Products {
		if strings.HasPrefix(p.Rewards, "frag_") && p.AccountWeeklyLimit > 0 && p.DailyLimit != 10000 {
			t.Fatal("weekly quota rewritten as daily", p)
		}
	}
}

func TestEconomy76ActualTrialFourMenusScheduledWeeklyQuota(t *testing.T) {
	f, actor := economy76Fixture(t)
	shop, err := f.s.AdminShop(f.ctx, "point")
	if err != nil {
		t.Fatal(err)
	}
	settings := shop.Settings
	settings.Unlimited = false
	if _, err = f.s.AdminSaveShopSettings(f.ctx, actor, "point", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version}); err != nil {
		t.Fatal(err)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var definition model.AdminActivityDefinitionDTO
	for _, d := range definitions {
		if d.ID == "shop_window_point" {
			definition = d
		}
	}
	if !definition.Enabled || !definition.Availability.Usable {
		t.Fatal("actual point shop not schedulable", definition)
	}
	reusePublish(t, f, actor, definition, 1, 7, "calendar")
	// Reload because publishing increments the definition/schedule versions.
	definitions, err = f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range definitions {
		if d.ID == definition.ID {
			definition = d
		}
	}
	reusePublish(t, f, actor, definition, 8, 7, "calendar")
	read := func(ctx context.Context) []*pb.ShopItem {
		t.Helper()
		out, e := f.s.p4Commerce(ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(5)}, 0, LocalPlayNow(ctx))
		if e != nil {
			t.Fatal(e)
		}
		return out[0].(*pb.SCShopProductList).GetShopItemList()
	}
	bound := serverClockBindTest(t, f, f.id)
	menus := map[int32]*pb.ShopItem{}
	for _, item := range read(bound) {
		if menus[item.GetPriority()/1000] == nil {
			menus[item.GetPriority()/1000] = item
		}
	}
	if len(menus) != 4 {
		t.Fatal("actual original trial catalogue missing menu", menus)
	}
	stamp := int64(762000)
	for menu := int32(1); menu <= 4; menu++ {
		item := menus[menu]
		count := int32(1)
		if menu == 4 {
			count = item.GetTotalLeftTimes()
			if count < 1 || count > 3 {
				t.Fatal("weekly original stock", item)
			}
		}
		before, _ := f.profile(t)
		request := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: item.Id, BuyCount: proto.Int32(count)}
		stamp++
		out, e := f.s.p4Commerce(bound, f.id, request, stamp, LocalPlayNow(bound))
		if e != nil {
			t.Fatal("menu purchase", menu, e)
		}
		after, _ := f.profile(t)
		if before.GetPoint()-after.GetPoint() != item.GetPrice()[0].GetNum()*count {
			t.Fatal("menu original price", menu, item)
		}
		retry, e := f.s.p4Commerce(bound, f.id, request, stamp, LocalPlayNow(bound))
		if e != nil || !proto.Equal(out[0], retry[0]) {
			t.Fatal("menu retry", menu, e)
		}
	}
	red := menus[4]
	request := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: red.Id, BuyCount: proto.Int32(1)}
	if _, err = f.s.p4Commerce(bound, f.id, request, 762010, LocalPlayNow(bound)); err == nil {
		t.Fatal("weekly stock exceeded same day")
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if _, err = f.s.p4Commerce(bound, f.id, request, 762011, LocalPlayNow(bound)); err == nil {
		t.Fatal("weekly stock reset as a daily quota")
	}
	for i := 0; i < 6; i++ {
		reuseClockDay(t, f, actor)
	}
	bound = serverClockBindTest(t, f, f.id)
	var next *pb.ShopItem
	for _, item := range read(bound) {
		if item.GetPriority()/1000 == 4 && p1SameResources(item.GetItem(), red.GetItem()) {
			next = item
			break
		}
	}
	if next == nil || next.GetTotalLeftTimes() != red.GetTotalTimes() {
		t.Fatal("next real weekly window missing original stock", next, red)
	}
	if _, err = f.s.p4Commerce(bound, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: next.Id, BuyCount: proto.Int32(1)}, 762012, LocalPlayNow(bound)); err != nil {
		t.Fatal("next scheduled week purchase", err)
	}
}

func TestEconomy76ActualCreditWeeklyPurchaseSurvivesRefreshAndGMDay(t *testing.T) {
	f, actor := economy76Fixture(t)
	for _, initialize := range []func(context.Context) error{f.s.InitializeLocalWallet, f.s.InitializeGlobalGameAccounts, f.s.InitializeAccountWallets} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	shop, err := f.s.AdminShop(f.ctx, "ggo")
	if err != nil {
		t.Fatal(err)
	}
	originalEnabled := map[string]bool{}
	for _, p := range shop.Items {
		originalEnabled[p.ID] = p.Enabled
	}
	var selected model.AdminShopProduct
	for _, p := range shop.Items {
		if p.Enabled && len(p.Rewards) == 1 && p.Rewards[0].ResourceID == "item_9711" {
			selected = p
			break
		}
	}
	if selected.ID == "" || selected.AccountWeeklyLimit != 7 || selected.Stock != -1 {
		t.Fatal("actual75 weekly7 source must have unlimited shared stock", selected)
	}
	for _, p := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if p.ID == selected.ID || !p.Enabled {
			continue
		}
		p.Enabled = false
		p.Stock = 1
		shop, err = f.s.AdminSaveShopProduct(f.ctx, actor, "ggo", p.ID, model.AdminShopProductSaveRequest{AdminShopProduct: p, Version: shop.Version}, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Reconstruct the released legacy Stock=1 term with the original admin API.
	selected.Stock = 1
	shop, err = f.s.AdminSaveShopProduct(f.ctx, actor, "ggo", selected.ID, model.AdminShopProductSaveRequest{AdminShopProduct: selected, Version: shop.Version}, false)
	if err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err = db.GetIns().First(&owner, f.id).Error; err != nil || owner.AccountID < 1 {
		t.Fatal("linked finite owner", err)
	}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	page, err := f.s.GGOShopCatalogue(ctx, f.id, owner.Session)
	if err != nil || len(page.Items) != 6 {
		t.Fatal(page, err)
	}
	first := model.GGOShopBuyRequest{PlayerID: f.id, ProductID: page.Items[0].ID, Version: page.Version, RequestID: uuid.NewString()}
	paid, err := f.s.GGOShopBuy(ctx, first, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paid.Catalogue.Items {
		if p.Remaining != 0 {
			t.Fatal("legacy source did not reproduce shared-stock exhaustion", p)
		}
	}
	t.Log("legacy Stock=1 reproduced: first purchase hides all unused sibling slots despite account-week quota7")
	// Restore the complete original growth catalogue before bulk stock-only repair.
	for _, p := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if !originalEnabled[p.ID] || p.Enabled {
			continue
		}
		p.Enabled = true
		shop, err = f.s.AdminSaveShopProduct(f.ctx, actor, "ggo", p.ID, model.AdminShopProductSaveRequest{AdminShopProduct: p, Version: shop.Version}, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	source, err := f.s.AdminOrdinaryShopSource(db.GetIns(), "ggo")
	if err != nil {
		t.Fatal(err)
	}
	var quoteRow db.P4CommerceState
	if err = db.GetIns().First(&quoteRow, "player_id=? AND rule_id=?", f.id, ggoQuoteRuleID(source.ID)).Error; err != nil {
		t.Fatal(err)
	}
	var quote ggoQuoteState
	if err = json.Unmarshal(quoteRow.Payload, &quote); err != nil {
		t.Fatal(err)
	}
	sourceID := quote.Slots[0].SourceProductID
	var oldStock db.P4CommerceStock
	if err = db.GetIns().First(&oldStock, "rule_id=? AND product_id=?", source.ID, sourceID).Error; err != nil || oldStock.Used != 1 {
		t.Fatal(oldStock, err)
	}
	var oldWeek db.GGOAccountWeekly
	if err = db.AccountsTable(db.GetIns(), constUtil.GGOAccountWeeklyTable).Where("account_id=? AND product_id=? AND week_start=?", owner.AccountID, sourceID, ggoAccountWeek(time.Now())).First(&oldWeek).Error; err != nil || oldWeek.Bought != 1 {
		t.Fatal(oldWeek, err)
	}
	before, g0 := f.profile(t)
	// Repair only the original mutable source stock, preserving accepted quote state.
	repaired := 0
	for _, p := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if !originalEnabled[p.ID] {
			continue
		}
		if p.Stock != 1 {
			t.Fatal("legacy bulk precondition Stock1", p)
		}
		expected := *shop
		expected.Items = append([]model.AdminShopProduct(nil), shop.Items...)
		for i := range expected.Items {
			if expected.Items[i].ID == p.ID {
				expected.Items[i].Stock = -1
			}
		}
		p.Stock = -1
		next, e := f.s.AdminSaveShopProduct(f.ctx, actor, "ggo", p.ID, model.AdminShopProductSaveRequest{AdminShopProduct: p, Version: shop.Version}, false)
		if e != nil {
			t.Fatal(e)
		}
		expected.Version = next.Version
		if !bytes.Equal(adminJSON(expected), adminJSON(next)) {
			t.Fatal("bulk update changed non-stock configuration", p.ID)
		}
		shop = next
		repaired++
	}
	if repaired != 46 {
		t.Fatal("expected all46 growth SKUs", repaired)
	}
	selected.Stock = -1
	t.Logf("bulk original Admin CAS modified only Stock across%d SKUs", repaired)
	fixed, err := f.s.GGOShopCatalogue(ctx, f.id, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	if fixed.Version == page.Version || fixed.Refresh.Generation != page.Refresh.Generation {
		t.Fatal("stock edit must change version without reroll", fixed)
	}
	var quoteAfter db.P4CommerceState
	if err = db.GetIns().First(&quoteAfter, "player_id=? AND rule_id=?", f.id, quoteRow.RuleID).Error; err != nil || !bytes.Equal(quoteAfter.Payload, quoteRow.Payload) {
		t.Fatal("admin repair changed bought slots/refresh counters", err)
	}
	var stockAfter db.P4CommerceStock
	db.GetIns().First(&stockAfter, "rule_id=? AND product_id=?", source.ID, sourceID)
	var weekAfter db.GGOAccountWeekly
	db.AccountsTable(db.GetIns(), constUtil.GGOAccountWeeklyTable).Where("account_id=? AND product_id=? AND week_start=?", owner.AccountID, sourceID, oldWeek.WeekStart).First(&weekAfter)
	after, g1 := f.profile(t)
	if stockAfter.Used != oldStock.Used || weekAfter.Bought != oldWeek.Bought || !proto.Equal(before, after) || !bytes.Equal(adminJSON(g0), adminJSON(g1)) {
		t.Fatal("repair reset history or assets")
	}
	replay, err := f.s.GGOShopBuy(ctx, first, owner.Session)
	if err != nil || !bytes.Equal(adminJSON(paid), adminJSON(replay)) {
		t.Fatal("old receipt changed after stock repair", err)
	}
	stale := first
	stale.RequestID = uuid.NewString()
	stale.ProductID = page.Items[1].ID
	if _, err = f.s.GGOShopBuy(ctx, stale, owner.Session); err == nil {
		t.Fatal("old pending quote accepted after eligibility change")
	}
	// Limit future draws to the same original product for deterministic weekly-cap checks.
	for _, p := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if p.ID == selected.ID || !p.Enabled {
			continue
		}
		p.Enabled = false
		shop, err = f.s.AdminSaveShopProduct(f.ctx, actor, "ggo", p.ID, model.AdminShopProductSaveRequest{AdminShopProduct: p, Version: shop.Version}, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err = f.s.GGOShopCatalogue(ctx, f.id, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	for count := int64(1); count < 7; count++ {
		var available *model.GGOShopItem
		for i := range page.Items {
			if page.Items[i].Remaining > 0 {
				available = &page.Items[i]
				break
			}
		}
		if available == nil {
			refresh, e := f.s.GGOShopRefresh(ctx, model.GGOShopRefreshRequest{PlayerID: f.id, Version: page.Version, RequestID: uuid.NewString()}, owner.Session)
			if e != nil {
				t.Fatal(e)
			}
			page = refresh.Catalogue
			for i := range page.Items {
				if page.Items[i].Remaining > 0 {
					available = &page.Items[i]
					break
				}
			}
		}
		if available == nil {
			t.Fatal("unused account-week allowance blocked by shared stock", count, page)
		}
		prior, gPrior := f.profile(t)
		walletPrior, e := walletReadBalance(db.GetIns(), f.id, false)
		if e != nil {
			t.Fatal(e)
		}
		req := model.GGOShopBuyRequest{PlayerID: f.id, ProductID: available.ID, Version: page.Version, RequestID: uuid.NewString()}
		result, e := f.s.GGOShopBuy(ctx, req, owner.Session)
		if e != nil {
			t.Fatal(count, e)
		}
		next, gNext := f.profile(t)
		walletNext, e := walletReadBalance(db.GetIns(), f.id, false)
		if e != nil {
			t.Fatal(e)
		}
		if simpleOwned(prior, gPrior, "item_9710")-simpleOwned(next, gNext, "item_9710") != selected.Cost.Amount || result.TerminalPointsAdded != selected.Rewards[0].Amount || walletNext.BalanceCents-walletPrior.BalanceCents != selected.Rewards[0].Amount*constUtil.GGOOverclockUnit {
			t.Fatal("weekly actual credit/reward delta", count)
		}
		again, e := f.s.GGOShopBuy(ctx, req, owner.Session)
		if e != nil || !bytes.Equal(adminJSON(result), adminJSON(again)) {
			t.Fatal("weekly buy replay", count, e)
		}
		page = result.Catalogue
	}
	for _, p := range page.Items {
		if p.Remaining != 0 {
			t.Fatal("weekly7 did not exhaust", p)
		}
	}
	refreshed, err := f.s.GGOShopRefresh(ctx, model.GGOShopRefreshRequest{PlayerID: f.id, Version: page.Version, RequestID: uuid.NewString()}, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	page = refreshed.Catalogue
	before, g0 = f.profile(t)
	if _, err = f.s.GGOShopBuy(ctx, model.GGOShopBuyRequest{PlayerID: f.id, ProductID: page.Items[0].ID, Version: page.Version, RequestID: uuid.NewString()}, owner.Session); err == nil {
		t.Fatal("eighth weekly purchase accepted")
	}
	after, g1 = f.profile(t)
	if !proto.Equal(before, after) || !bytes.Equal(adminJSON(g0), adminJSON(g1)) {
		t.Fatal("eighth purchase changed assets")
	}
	// A separately registered account must retain its independent weekly allowance.
	account, err := registerGlobalGameAccount(ctx, "e76-"+uuid.NewString(), "isolated-economy76-pass", "独立账号")
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoginAccount(ctx, account.Username, "isolated-economy76-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, second.Id, "独立商店买家", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, second.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		_, e := f.s.GrantFixedReward(p, g, "item_9710,1000")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var secondOwner db.Profile
	if err = db.GetIns().First(&secondOwner, second.Id).Error; err != nil || secondOwner.AccountID == owner.AccountID {
		t.Fatal("independent registration", err)
	}
	other, err := f.s.GGOShopCatalogue(ctx, second.Id, secondOwner.Session)
	if err != nil || other.Items[0].Remaining != 1 {
		t.Fatal("other account blocked by first account's stock", other, err)
	}
	if _, err = f.s.GGOShopBuy(ctx, model.GGOShopBuyRequest{PlayerID: second.Id, ProductID: other.Items[0].ID, Version: other.Version, RequestID: uuid.NewString()}, secondOwner.Session); err != nil {
		t.Fatal("second account original purchase", err)
	}
	reuseClockDay(t, f, actor)
	bound := serverClockBindTest(t, f, f.id)
	page, err = f.s.GGOShopCatalogue(bound, f.id, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range page.Items {
		if p.Remaining != 0 {
			t.Fatal("GM day reopened real account-week quota", p)
		}
	}
	t.Logf("original weekly terms proven: %s reward%v price%v; first account7 accepted, eighth denied, second account independently accepted; no quota/quote reset", selected.Name, selected.Rewards, selected.Cost)
}

func TestEconomy76ActualOrdinaryScoreAndMemoryPurchaseTerms(t *testing.T) {
	f, _ := economy76Fixture(t)
	now := time.Now()
	for i, spec := range []struct {
		kind     int32
		resource string
		currency string
	}{{1, "item_005", "diamond"}, {7, "item_006", "score"}} {
		var out []proto.Message
		var err error
		if spec.kind == 1 {
			out, err = f.s.shopProducts(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(1)})
		} else {
			out, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(spec.kind)}, 0, now)
		}
		if err != nil {
			t.Fatal(err)
		}
		var chosen *pb.ShopItem
		for _, item := range out[0].(*pb.SCShopProductList).GetShopItemList() {
			for _, reward := range item.GetItem() {
				if spec.kind == 1 && reward.GetId() == "coin" {
					t.Fatal("ordinary shop still sells gold")
				}
				if reward.GetId() == spec.resource {
					chosen = item
				}
			}
		}
		if chosen == nil || chosen.GetPrice()[0].GetId() != spec.currency {
			t.Fatalf("actual ordinary/score product %v chosen=%v catalogue=%v", spec, chosen, out)
		}
		before, g0 := f.profile(t)
		request := &pb.CSShopBuy{ShopType: proto.Int32(spec.kind), ShopItemId: chosen.Id, BuyCount: proto.Int32(1)}
		stamp := int64(763000 + i)
		var paid []proto.Message
		if spec.kind == 1 {
			paid, err = f.s.shopBuy(f.ctx, f.id, request, stamp)
		} else {
			paid, err = f.s.p4Commerce(f.ctx, f.id, request, stamp, now)
		}
		if err != nil {
			t.Fatal(err)
		}
		after, g1 := f.profile(t)
		if simpleOwned(after, g1, spec.resource)-simpleOwned(before, g0, spec.resource) != int64(chosen.GetItem()[0].GetNum()) {
			t.Fatal("actual reward quantity", chosen)
		}
		var spent int64
		if spec.kind == 1 {
			spent = before.GetDiamond() - after.GetDiamond()
		} else {
			spent = int64(before.GetScore() - after.GetScore())
		}
		t.Logf("actual shop type%d resource%s amount%d currency%s spent%d", spec.kind, spec.resource, chosen.GetItem()[0].GetNum(), spec.currency, spent)
		if spent != int64(chosen.GetPrice()[0].GetNum()) {
			t.Fatal("actual currency price", spent, chosen)
		}
		var replay []proto.Message
		if spec.kind == 1 {
			replay, err = f.s.shopBuy(f.ctx, f.id, request, stamp)
		} else {
			replay, err = f.s.p4Commerce(f.ctx, f.id, request, stamp, now)
		}
		if err != nil || !proto.Equal(paid[0], replay[0]) {
			t.Fatal("ordinary/score replay", err)
		}
	}
	out, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	items := out[0].(*pb.SCRefreshShopQuery).GetShopItemList()
	if len(items) != 6 {
		t.Fatal("actual memory slot count", len(items))
	}
	chosen := items[0]
	if chosen.GetPrice()[0].GetId() != "light" {
		t.Fatal("memory currency", chosen)
	}
	before, _ := f.profile(t)
	request := &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: chosen.Id}
	paid, err := f.s.p4Commerce(f.ctx, f.id, request, 763010, now)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if before.GetLight()-after.GetLight() != chosen.GetPrice()[0].GetNum() {
		t.Fatal("memory quote not charged")
	}
	replay, err := f.s.p4Commerce(f.ctx, f.id, request, 763010, now)
	if err != nil || !proto.Equal(paid[0], replay[0]) {
		t.Fatal("memory original replay", err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, request, 763011, now); err == nil {
		t.Fatal("bought memory slot bought twice")
	}
}

func TestEconomy76ActualGuildShopHundredContributionAndBadgeCost(t *testing.T) {
	f, _ := economy76Fixture(t)
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{&db.P13Guild{ID: "economy76-guild", Name: "有限会员前置", Level: 1}, &db.P13GuildMember{PlayerID: f.id, GuildID: "economy76-guild", Contribution: 1000}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Updates(map[string]any{"management_enabled": true, "economy_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Contribution = proto.Int32(1000)
		_, e := f.s.GrantFixedReward(p, g, "item_311,500")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	out, err := f.s.p4GuildShop(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(8)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	items := out[0].(*pb.SCShopProductList).GetShopItemList()
	if len(items) != 6 {
		t.Fatal("75 guild catalogue count", len(items))
	}
	chosen := items[0]
	prices := map[string]int32{}
	for _, r := range chosen.GetExchangeList() {
		prices[r.GetId()] = r.GetNum()
	}
	t.Logf("actual guild original dual costs: %v", prices)
	if prices["contribution"] != 100 || prices["item_311"] < 10 {
		t.Fatal("75 original dual price", chosen)
	}
	request := &pb.CSShopBuy{ShopType: proto.Int32(8), ShopItemId: chosen.Id, BuyCount: proto.Int32(1)}
	before, g0 := f.profile(t)
	paid, err := f.s.p4GuildShop(f.ctx, f.id, request, 764001, now)
	if err != nil {
		t.Fatal(err)
	}
	after, g1 := f.profile(t)
	if before.GetContribution()-after.GetContribution() != 100 || simpleOwned(before, g0, "item_311")-simpleOwned(after, g1, "item_311") != int64(prices["item_311"]) {
		t.Fatal("guild actual contribution/badge mismatch")
	}
	replay, err := f.s.p4GuildShop(f.ctx, f.id, request, 764001, now)
	if err != nil || !proto.Equal(paid[0], replay[0]) {
		t.Fatal("guild purchase replay", err)
	}
	if _, err = f.s.p4GuildShop(f.ctx, f.id, request, 764002, now); err == nil {
		t.Fatal("guild daily 1 limit bypassed")
	}
}

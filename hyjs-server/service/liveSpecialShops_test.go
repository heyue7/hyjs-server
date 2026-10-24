package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Finite paid eligibility and stock resources isolate actual native purchases;
// operational changes go through the real typed admin endpoint service.
func TestLiveSpecialShopsVIPAndPetCostsQuotesLimitsAndCold(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializePayments, f.s.InitializeP9Pets, f.s.InitializeP9PetRecycle} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Vip = proto.Int32(6)
		p.Coinpet = proto.Int64(1000)
		p.Diamond = proto.Int64(1000)
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_001"), Num: proto.Int32(100)}, {Id: proto.String("frag_hero_001"), Num: proto.Int32(100)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Historical confirmed eligibility is a declared prerequisite; the payment
	// pipeline itself is covered by the separately retained original P3 checks.
	if err := db.GetIns().Create(&db.PaymentOrder{ID: "live-shop-paid-proof", PlayerID: f.id, RequestKey: 1, RequestHash: "fixture", ProductID: "fixture", Snapshot: `{"charge":1}`, Status: constUtil.P3PaymentConfirmed, Cents: 100, ConfirmedAt: now.Add(-time.Second).UnixMilli(), CreatedAt: now.Add(-time.Minute).UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli(), Source: constUtil.P3PaymentSource}).Error; err != nil {
		t.Fatal(err)
	}
	save := func(key string, p model.AdminShopProduct, create bool) *model.AdminShopResponse {
		t.Helper()
		current, e := f.s.AdminShop(f.ctx, key)
		if e != nil {
			t.Fatal(e)
		}
		next, e := f.s.AdminSaveShopProduct(f.ctx, actor, key, p.ID, model.AdminShopProductSaveRequest{AdminShopProduct: p, Version: current.Version}, create)
		if e != nil {
			t.Fatal(key, e)
		}
		return next
	}
	vip := model.AdminShopProduct{Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 1}}, Cost: model.AdminResourceAmount{ResourceID: "item_001", Amount: 3}, MinVip: 1, Enabled: true, Source: "finite operator live quote"}
	result := save("vip", vip, true)
	for _, v := range result.Items {
		if v.Source == vip.Source {
			vip = v
		}
	}
	listVIP := func() *pb.ShopItem {
		t.Helper()
		messages, e := f.s.vipShop(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(2)}, now)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range messages[0].(*pb.SCShopProductList).GetShopItemList() {
			if item.GetPrice()[0].GetId() == vip.Cost.ResourceID {
				return item
			}
		}
		t.Fatal("live VIP missing")
		return nil
	}
	old := listVIP()
	vip.Cost = model.AdminResourceAmount{ResourceID: "frag_hero_001", Amount: 4}
	vip.Rewards[0].Amount = 2
	save("vip", vip, false)
	before, _ := f.profile(t)
	if _, e := f.s.vipShop(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String(old.GetId()), BuyCount: proto.Int32(1)}, now); !errors.Is(e, constUtil.ErrShopQuoteChanged) {
		t.Fatal("stale VIP charged current quote", e)
	}
	after, _ := f.profile(t)
	if before.GetDiamond() != after.GetDiamond() {
		t.Fatal("stale quote changed wallet")
	}
	fresh := listVIP()
	if fresh.GetPrice()[0].GetId() != "frag_hero_001" || fresh.GetPrice()[0].GetNum() != 4 || fresh.GetItem()[0].GetNum() != 2 {
		t.Fatal("VIP edit not live", fresh)
	}
	if _, e := f.s.vipShop(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String(fresh.GetId()), BuyCount: proto.Int32(1)}, now); e != nil {
		t.Fatal("VIP generic material", e)
	}
	_, g := f.profile(t)
	p, _ := f.profile(t)
	if simpleOwned(p, g, "frag_hero_001") != 96 || simpleOwned(p, g, "item_005") != 2 {
		t.Fatal("VIP actual costs/reward")
	}
	vip.Cost.Amount = 7
	save("vip", vip, false)
	if listVIP().GetCanBuy() != 0 {
		t.Fatal("VIP edit reset daily claim")
	}

	// Six independent positions use a declared pool of four fragments and two materials.
	// Existing offers retain their accepted amount/cost until an explicit refresh.
	var rule db.P9PetMarketRule
	if err := db.GetIns().First(&rule, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
		t.Fatal(err)
	}
	var cfg p9MarketConfig
	if err := json.Unmarshal(rule.Input, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Slots = 6
	cfg.Offers = []p9MarketOffer{
		{ID: "live-frag1", Resource: "frag_pet_001", Amount: 1, Currency: "item_001", Price: 2, Stock: 3, Weight: 1},
		{ID: "live-frag2", Resource: "frag_pet_002", Amount: 1, Currency: "item_001", Price: 2, Stock: 3, Weight: 1},
		{ID: "live-frag3", Resource: "frag_pet_003", Amount: 1, Currency: "item_001", Price: 2, Stock: 3, Weight: 1},
		{ID: "live-frag4", Resource: "frag_pet_004", Amount: 1, Currency: "item_001", Price: 2, Stock: 3, Weight: 1},
		{ID: "live-pet", Resource: "item_124", Amount: 1, Currency: "item_001", Price: 2, Stock: 3, Weight: 1},
		{ID: "live-pet2", Resource: "item_005", Amount: 1, Currency: "item_001", Price: 2, Stock: 3, Weight: 1},
	}
	if err := db.GetIns().Model(&rule).Updates(map[string]any{"enabled": true, "input": adminJSON(cfg)}).Error; err != nil {
		t.Fatal(err)
	}
	listPet := func() *pb.SCPetShopProductList {
		t.Helper()
		messages, e := f.s.p9PetShopList(f.ctx, f.id)
		if e != nil {
			t.Fatal(e)
		}
		list := messages[0].(*pb.SCPetShopProductList)
		if len(list.GetShopItemList()) != 6 {
			t.Fatal("pet selected positions", len(list.GetShopItemList()))
		}
		fragments := 0
		seen := map[string]bool{}
		for _, item := range list.ShopItemList {
			if seen[item.GetId()] {
				t.Fatal("duplicate slot identity")
			}
			seen[item.GetId()] = true
			if strings.HasPrefix(item.Item[0].GetId(), "frag_pet_") {
				fragments++
			}
		}
		if fragments < 1 || fragments > 4 {
			t.Fatal("fragment/material split", fragments)
		}
		return list
	}
	findSlot := func(list *pb.SCPetShopProductList, sku string) *pb.ShopItem {
		t.Helper()
		for _, item := range list.ShopItemList {
			if item.GetId() == sku {
				return item
			}
		}
		t.Fatal("accepted slot disappeared", sku)
		return nil
	}
	initial := listPet()
	var petOld *pb.ShopItem
	for _, item := range initial.ShopItemList {
		if !strings.HasPrefix(item.Item[0].GetId(), "frag_pet_") {
			petOld = item
			break
		}
	}
	if petOld == nil {
		t.Fatal("material positions missing")
	}
	targetResource := petOld.Item[0].GetId()
	p, g = f.profile(t)
	targetBefore := simpleOwned(p, g, targetResource)
	shop, e := f.s.AdminShop(f.ctx, "pet-market")
	if e != nil {
		t.Fatal(e)
	}
	var pet model.AdminShopProduct
	for _, item := range shop.Items {
		if item.Rewards[0].ResourceID == targetResource {
			pet = item
			break
		}
	}
	if pet.ID == "" {
		t.Fatal("selected product absent from admin catalogue")
	}
	pet.Cost = model.AdminResourceAmount{ResourceID: "frag_hero_001", Amount: 3}
	pet.Rewards[0].Amount = 2
	save("pet-market", pet, false)
	unchanged := findSlot(listPet(), petOld.GetId())
	if !proto.Equal(unchanged, petOld) {
		t.Fatal("config edit replaced accepted pet quote", unchanged)
	}
	buyOld := &pb.CSPetShopBuy{ShopItemId: proto.String(petOld.GetId()), BuyCount: proto.Int32(1)}
	bought, e := f.s.p9PetShopBuy(f.ctx, f.id, buyOld, 98001)
	if e != nil {
		t.Fatal("accepted old quote", e)
	}
	originalBuy := bought[0].(*pb.SCPetShopBuy)
	if findSlot(listPet(), petOld.GetId()).GetCanBuy() != 2 {
		t.Fatal("pet stock accounting")
	}
	var receipt db.P9PetReceipt
	if e := db.GetIns().First(&receipt, "player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P9MarketBuyOperation, 98001).Error; e != nil {
		t.Fatal(e)
	}
	var oldState db.P9PetMarketState
	if e := db.GetIns().First(&oldState, "player_id=?", f.id).Error; e != nil {
		t.Fatal(e)
	}
	pet.Stock = 2
	pet.Cost.Amount = 5
	save("pet-market", pet, false)
	got := findSlot(listPet(), petOld.GetId())
	if got.GetCanBuy() != 1 || got.Price[0].GetId() != "item_001" || got.Price[0].GetNum() != 2 || got.Item[0].GetNum() != 1 {
		t.Fatal("live stock or frozen quote", got)
	}
	var state db.P9PetMarketState
	if e := db.GetIns().First(&state, "player_id=?", f.id).Error; e != nil {
		t.Fatal(e)
	}
	if state.Generation != oldState.Generation || state.RefreshUsed != oldState.RefreshUsed || !bytes.Equal(state.Selected, oldState.Selected) || !bytes.Equal(state.Bought, oldState.Bought) {
		t.Fatal("edit refreshed or reset purchased slots")
	}
	f.restart(t)
	if listVIP().GetCanBuy() != 0 || findSlot(listPet(), petOld.GetId()).GetCanBuy() != 1 {
		t.Fatal("cold shop lost quotas")
	}
	retry, e := f.s.p9PetShopBuy(f.ctx, f.id, buyOld, 98001)
	if e != nil || !proto.Equal(retry[0].(*pb.SCPetShopBuy).Get, originalBuy.Get) || !p1SameResources(retry[0].(*pb.SCPetShopBuy).Cost, originalBuy.Cost) {
		t.Fatal("old quote UUID lost successful terms", e)
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "frag_hero_001") != 96 || simpleOwned(p, g, "item_001") != 98 || simpleOwned(p, g, targetResource) != targetBefore+1 {
		t.Fatal("old fixed quote/retry did not persist exact cost and reward")
	}
	// Disable only the unselected material so the explicit refresh deterministically
	// includes the edited material; four healthy fragment products remain enabled.
	shop, e = f.s.AdminShop(f.ctx, "pet-market")
	if e != nil {
		t.Fatal(e)
	}
	for _, other := range shop.Items {
		if !strings.HasPrefix(other.Rewards[0].ResourceID, "frag_pet_") && other.ID != pet.ID {
			other.Enabled = false
			save("pet-market", other, false)
		}
	}
	freshMessages, e := f.s.p9PetShopRefresh(f.ctx, f.id, &pb.CSRefreshPetShop{}, 98003)
	if e != nil {
		t.Fatal("explicit pet refresh", e)
	}
	refreshed := freshMessages[0].(*pb.SCPetShopProductList)
	if refreshed.GetRefreshTimes() != initial.GetRefreshTimes()-1 {
		t.Fatal("refresh counter not consumed once")
	}
	var petFresh *pb.ShopItem
	for _, item := range refreshed.ShopItemList {
		if item.Item[0].GetId() == targetResource {
			petFresh = item
			break
		}
	}
	if petFresh == nil || petFresh.GetId() == petOld.GetId() || petFresh.Price[0].GetId() != "frag_hero_001" || petFresh.Price[0].GetNum() != 5 || petFresh.Item[0].GetNum() != 2 || petFresh.GetCanBuy() != 2 {
		t.Fatal("new refresh ignored current configured quote", petFresh)
	}
	if _, e = f.s.p9PetShopBuy(f.ctx, f.id, buyOld, 98002); e == nil {
		t.Fatal("old generation accepted new purchase")
	}
	if _, e = f.s.p9PetShopBuy(f.ctx, f.id, buyOld, 98001); e == nil {
		t.Fatal("old-generation receipt projected stale inventory")
	}
	buyNew := &pb.CSPetShopBuy{ShopItemId: proto.String(petFresh.GetId()), BuyCount: proto.Int32(1)}
	freshBuy, e := f.s.p9PetShopBuy(f.ctx, f.id, buyNew, 98004)
	if e != nil {
		t.Fatal("new current generic material quote", e)
	}
	freshResult := freshBuy[0].(*pb.SCPetShopBuy)
	f.restart(t)
	if listVIP().GetCanBuy() != 0 || findSlot(listPet(), petFresh.GetId()).GetCanBuy() != 1 {
		t.Fatal("cold refreshed inventory reset purchases")
	}
	freshRetry, e := f.s.p9PetShopBuy(f.ctx, f.id, buyNew, 98004)
	if e != nil || !proto.Equal(freshRetry[0].(*pb.SCPetShopBuy).Get, freshResult.Get) || !p1SameResources(freshRetry[0].(*pb.SCPetShopBuy).Cost, freshResult.Cost) {
		t.Fatal("new UUID cold receipt changed", e)
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "frag_hero_001") != 91 || simpleOwned(p, g, "item_001") != 98 || simpleOwned(p, g, targetResource) != targetBefore+3 || p.GetDiamond() != 850 {
		t.Fatal("generic material/reward/refresh debit not persisted")
	}
	var saved db.P9PetReceipt
	if e := db.GetIns().First(&saved, "player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P9MarketBuyOperation, 98001).Error; e != nil || !bytes.Equal(saved.Response, receipt.Response) {
		t.Fatal("refresh overwrote historical successful receipt", e)
	}

}

// A single original war-shop actor is exercised after the existing bounded
// match fixture: stable stock, actual normal materials, live edits and receipts.
func TestLiveUnionFightShopMaterialsQuotesStockAndCold(t *testing.T) {
	f, ids, _, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "finite live shop actor"}).Error; err != nil {
		t.Fatal(err)
	}
	source := db.P13UnionFightShopSource{Day: match.Day, ShopItemID: "live-material", Enabled: true, ItemID: "ufitem_016", CostResource: "item_001", Price: 2, Quantity: 1, BuyCount: 3, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "finite operator actual materials"}
	if err := db.GetIns().Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	id := ids[0]
	if _, err := f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	npc := "3"
	if actor.Camp == 1 {
		npc = "6"
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	point, err := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
	if err != nil {
		t.Fatal(err)
	}
	// Accumulate legal walking time on the existing server-clock fixture.
	clock.Add(60000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_001"), Num: proto.Int32(10)}, {Id: proto.String("frag_hero_001"), Num: proto.Int32(10)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	list := func() *pb.InstUFShopItem {
		t.Helper()
		messages, e := f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(npc)}, now())
		if e != nil {
			t.Fatal(e)
		}
		items := messages[0].(*pb.SCUFShopList).GetItems()
		if len(items) != 1 {
			t.Fatal("live UF source missing")
		}
		return items[0]
	}
	buy := func(sku string, stamp int64) ([]proto.Message, error) {
		return f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(sku), NpcId: proto.String(npc)}, stamp, now())
	}
	old := list()
	if !strings.HasPrefix(old.GetShopItemId(), "ufq:item_001:") {
		t.Fatal("UF missing explicit material quote", old)
	}
	first, err := buy(old.GetShopItemId(), 99001)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 4 || first[1].(*pb.SCItemList).GetItemList()[0].GetNum() != 8 {
		t.Fatal("UF actual material debit/current native sync", first)
	}
	// Update the real operational row inside the same accepted match; stock must
	// account for the already consumed unit even while capacity is reduced.
	source.CostResource = "frag_hero_001"
	source.Price = 3
	source.BuyCount = 2
	if err = db.GetIns().Save(&source).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = buy(old.GetShopItemId(), 99002); !errors.Is(err, constUtil.ErrShopQuoteChanged) {
		t.Fatal("UF stale quote charged", err)
	}
	current := list()
	if current.GetLeftBuyCount() != 1 || !strings.HasPrefix(current.GetShopItemId(), "ufq:frag_hero_001:") {
		t.Fatal("UF live edit reset stock or hid material", current)
	}
	second, err := buy(current.GetShopItemId(), 99003)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range second[1].(*pb.SCItemList).GetItemList() {
		if v.GetId() == "frag_hero_001" && v.GetNum() == 7 {
			found = true
		}
	}
	if !found {
		t.Fatal("UF material persisted/synced")
	}
	if list().GetLeftBuyCount() != 0 {
		t.Fatal("UF second unit stock")
	}
	f.restart(t)
	replay, err := buy(old.GetShopItemId(), 99001)
	if err != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("UF old receipt lost after live edit/cold", err)
	}
	var stock db.P13UnionFightShopStock
	if err = db.GetIns().First(&stock, "match_id=? AND player_id=? AND shop_item_id=?", match.ID, id, source.ShopItemID).Error; err != nil || stock.LeftCount != 0 || stock.Capacity != 2 {
		t.Fatal("UF stable used units/cold", stock, err)
	}
}

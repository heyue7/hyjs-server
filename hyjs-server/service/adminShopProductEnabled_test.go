package service

import (
	"bytes"
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

func TestAdminShopProductEnabledCompatibility(t *testing.T) {
	for _, definition := range AdminShopDefinitions() {
		found := false
		for _, field := range definition.Fields {
			found = found || field == "enabled"
		}
		if !found {
			t.Fatalf("%s has no product switch", definition.ID)
		}
	}
	var old p4Product
	if err := json.Unmarshal([]byte(`{"id":"old"}`), &old); err != nil || !old.isEnabled() {
		t.Fatal("legacy P4 product disabled", err)
	}
	var suspended p4Product
	if err := json.Unmarshal([]byte(`{"id":"old","enabled":false}`), &suspended); err != nil || suspended.isEnabled() {
		t.Fatal("explicit stop ignored", err)
	}
	var first, second p9MarketOffer
	if err := json.Unmarshal([]byte(`{"id":"legacy"}`), &first); err != nil || !first.isEnabled() {
		t.Fatal("legacy pet offer disabled", err)
	}
	if err := json.Unmarshal([]byte(`{"id":"legacy","enabled":true}`), &second); err != nil || !p9MarketSameOffer(first, second) {
		t.Fatal("logical true compatibility", err)
	}
}

func enabledShopToggle(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, key, productID string, enabled bool) {
	t.Helper()
	page, err := f.s.AdminShop(f.ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID != productID {
			continue
		}
		item.Enabled = enabled
		updated, err := f.s.AdminSaveShopProduct(f.ctx, actor, key, item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: page.Version}, false)
		if err != nil {
			t.Fatal("status save", err)
		}
		for _, actual := range updated.Items {
			if actual.ID == item.ID && actual.Enabled == enabled {
				return
			}
		}
		t.Fatal("saved state missing from admin response")
	}
	t.Fatal("product missing", key, productID)
}

// The original buy, list, and same-stamp receipt paths exercise both variable-price
// and selected-slot catalogues; only an operator status flag changes between them.
func TestAdminShopProductEnabledP4ReceiptSlotsAndCold(t *testing.T) {
	for _, key := range []string{"ordinary", "memory"} {
		t.Run(key, func(t *testing.T) {
			f, actor := adminModesFixture(t)
			p4Fund(t, f)
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Coin = proto.Int64(100000000); return nil }); err != nil {
				t.Fatal(err)
			}
			page, err := f.s.AdminShop(f.ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", page.Settings.PeriodID).Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			shopType := int32(1)
			var query proto.Message = &pb.CSShopProductList{ShopType: proto.Int32(1)}
			if key == "memory" {
				shopType = 3
				query = &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}
			}
			list := func() []*pb.ShopItem {
				out, err := f.s.p4Commerce(f.ctx, f.id, query, 0, now)
				if err != nil {
					t.Fatal(err)
				}
				switch value := out[0].(type) {
				case *pb.SCShopProductList:
					return value.ShopItemList
				case *pb.SCRefreshShopQuery:
					return value.ShopItemList
				}
				t.Fatal("wrong native response")
				return nil
			}
			initial := list()
			if len(initial) == 0 {
				t.Fatal("no original product")
			}
			target := initial[0]
			if key == "ordinary" {
				for _, item := range initial {
					if item.GetId() == "itemShopId_1" {
						target = item
					}
				}
			}
			stable := target.GetId()
			var buy proto.Message = &pb.CSShopBuy{ShopType: proto.Int32(shopType), ShopItemId: proto.String(stable), BuyCount: proto.Int32(1)}
			if key == "memory" {
				buy = &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: proto.String(stable)}
			}
			first, err := f.s.p4Commerce(f.ctx, f.id, buy, 9701, now)
			if err != nil {
				t.Fatal("original buy", err)
			}
			var before db.P4CommerceState
			if err = db.GetIns().First(&before, "player_id=? AND rule_id=?", f.id, page.Settings.PeriodID).Error; err != nil {
				t.Fatal(err)
			}
			enabledShopToggle(t, f, actor, key, "period:"+stable, false)
			for _, item := range list() {
				_, id, quoted := adminLiveShopParseSKU(item.GetId())
				if !quoted {
					id = item.GetId()
				}
				if id == stable {
					t.Fatal("suspended product visible")
				}
			}
			if _, err = f.s.p4Commerce(f.ctx, f.id, buy, 9702, now); err == nil {
				t.Fatal("suspended old quote purchased")
			}
			replay, err := f.s.p4Commerce(f.ctx, f.id, buy, 9701, now)
			if err != nil || !proto.Equal(first[0], replay[0]) {
				t.Fatal("successful receipt lost after stop", err)
			}
			enabledShopToggle(t, f, actor, key, "period:"+stable, true)
			var after db.P4CommerceState
			if err = db.GetIns().First(&after, "player_id=? AND rule_id=?", f.id, page.Settings.PeriodID).Error; err != nil || !bytes.Equal(before.Payload, after.Payload) {
				t.Fatal("toggle reset slot/version/quota", err)
			}
			var cfg db.P4CommerceRule
			db.GetIns().First(&cfg, "id=?", page.Settings.PeriodID)
			var rules p4CommerceRules
			json.Unmarshal(cfg.Rules, &rules)
			if key == "ordinary" {
				for _, product := range rules.Products {
					if product.ID == stable && !product.TierID {
						t.Fatal("switch changed original tier price")
					}
				}
			}
			f.restart(t)
			restored := list()
			found := false
			for _, item := range restored {
				_, id, quoted := adminLiveShopParseSKU(item.GetId())
				if !quoted {
					id = item.GetId()
				}
				if id == stable {
					found = true
					if item.GetDayLeftTimes() != target.GetDayLeftTimes()-1 {
						t.Fatal("re-enable replenished quota")
					}
				}
			}
			if !found {
				t.Fatal("re-enabled original slot missing after cold start")
			}
		})
	}
}

func TestAdminShopProductEnabledPetReceiptInventoryAndCold(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.initializeP9Market(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P9PetReceipt{}); err != nil {
		t.Fatal(err)
	}
	var live db.P9PetMarketRule
	if err := db.GetIns().First(&live, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
		t.Fatal(err)
	}
	var cfg p9MarketConfig
	json.Unmarshal(live.Input, &cfg)
	for _, offer := range cfg.Offers {
		if offer.ID == "item_124" {
			cfg.Offers = []p9MarketOffer{offer}
			break
		}
	}
	if len(cfg.Offers) != 1 {
		t.Fatal("missing original material offer")
	}
	cfg.Slots = 1
	cfg.Offers[0].Stock = 2
	if err := db.GetIns().Model(&live).Updates(map[string]interface{}{"enabled": true, "input": adminJSON(cfg)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Coinpet = proto.Int64(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	listing := func() *pb.SCPetShopProductList {
		out, err := f.s.p9PetShopList(f.ctx, f.id)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCPetShopProductList)
	}
	first := listing()
	buy := &pb.CSPetShopBuy{ShopItemId: first.ShopItemList[0].Id, BuyCount: proto.Int32(1)}
	out, err := f.s.p9PetShopBuy(f.ctx, f.id, buy, 9801)
	if err != nil {
		t.Fatal("original pet buy", err)
	}
	original := out[0].(*pb.SCPetShopBuy)
	var before db.P9PetMarketState
	db.GetIns().First(&before, "player_id=?", f.id)
	var receipt db.P9PetReceipt
	db.GetIns().First(&receipt, "player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P9MarketBuyOperation, 9801)
	enabledShopToggle(t, f, actor, "pet-market", "item_124", false)
	if len(listing().ShopItemList) != 0 {
		t.Fatal("suspended pet product visible")
	}
	if _, err = f.s.p9PetShopBuy(f.ctx, f.id, buy, 9802); err == nil {
		t.Fatal("suspended pet quote purchased")
	}
	replay, err := f.s.p9PetShopBuy(f.ctx, f.id, buy, 9801)
	if err != nil {
		t.Fatal("same-epoch successful pet receipt blocked", err)
	}
	result := replay[0].(*pb.SCPetShopBuy)
	if !proto.Equal(result.Get, original.Get) || !p1SameResources(result.Cost, original.Cost) || len(result.ShopItemList) != 0 {
		t.Fatal("receipt terms changed or stale product revived")
	}
	enabledShopToggle(t, f, actor, "pet-market", "item_124", true)
	f.restart(t)
	restored := listing()
	if len(restored.ShopItemList) != 1 || restored.ShopItemList[0].GetDayLeftTimes() != 1 {
		t.Fatal("re-enable refilled pet stock")
	}
	var after db.P9PetMarketState
	db.GetIns().First(&after, "player_id=?", f.id)
	if before.Generation != after.Generation || before.RefreshUsed != after.RefreshUsed || before.WeekKey != after.WeekKey || !bytes.Equal(before.Bought, after.Bought) {
		t.Fatal("toggle changed generation/refresh/bought")
	}
	var saved db.P9PetReceipt
	db.GetIns().First(&saved, "player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P9MarketBuyOperation, 9801)
	if !bytes.Equal(receipt.Response, saved.Response) {
		t.Fatal("operational projection rewrote persistent receipt")
	}
	p, g := f.profile(t)
	if p.GetCoinpet() != 80 || simpleOwned(p, g, "item_124") != 10 {
		t.Fatal("toggle/retry duplicated fee or reward")
	}
	if !strings.HasPrefix(restored.ShopItemList[0].GetId(), "p9shop:"+after.WeekKey+":") {
		t.Fatal("real pet SKU lost")
	}
}

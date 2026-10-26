package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestAdminScheduledOrdinaryShopOriginalConsumerAndCold(t *testing.T) {
	f, actor := reuseFixture(t, false)
	p4Fund(t, f)
	// This isolated original catalogue is operator-enabled before scheduling.
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "ordinary").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminShopAvailability{}).Where("shop_key=?", "ordinary").Update("unlimited", false).Error; err != nil {
		t.Fatal(err)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var definition model.AdminActivityDefinitionDTO
	for _, d := range definitions {
		if d.ID == "shop_window_ordinary" {
			definition = d
		}
	}
	if !definition.Enabled || !definition.Availability.Usable || definition.Form.Options.ShopKey != "ordinary" {
		t.Fatal("limited real shop missing from schedule library", definition)
	}
	source, err := f.s.AdminOrdinaryShopSource(db.GetIns().WithContext(f.ctx), "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(source)
	slot := reusePublish(t, f, actor, definition, 2, 1, "calendar")
	bound := serverClockBindTest(t, f, f.id)
	query := &pb.CSShopProductList{ShopType: proto.Int32(1)}
	if closed, e := f.s.p4Commerce(bound, f.id, query, 0, LocalPlayNow(bound)); e != nil || len(closed) != 1 || len(closed[0].(*pb.SCShopProductList).GetShopItemList()) != 0 {
		t.Fatal("limited shop opened before scheduled date", e)
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	messages, err := f.s.p4Commerce(bound, f.id, query, 0, LocalPlayNow(bound))
	if err != nil || len(messages) == 0 {
		t.Fatal("original ordinary shop could not consume its scheduled catalogue", err)
	}
	var native db.P4CommerceRule
	_, key, _ := strings.Cut(slot.ConsumerID, ":")
	if err = db.GetIns().First(&native, "id=?", key).Error; err != nil || native.ShopType != source.ShopType || native.Kind != source.Kind || !strings.HasPrefix(native.ID, "shop_schedule_ordinary_") {
		t.Fatal("scheduled shop substituted a sale consumer", native, err)
	}
	before, _ := f.profile(t)
	buy := &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_1"), BuyCount: proto.Int32(1)}
	paid, err := f.s.p4Commerce(bound, f.id, buy, 93001, LocalPlayNow(bound))
	if err != nil {
		t.Fatal("original tier-price shop purchase", err)
	}
	after, _ := f.profile(t)
	expected, e := f.s.p4TierCost("itemShopId_1", 1, 1)
	if e != nil || before.GetDiamond()-after.GetDiamond() != int64(expected.GetNum()) {
		t.Fatal("scheduled original price changed")
	}
	var state db.P4CommerceState
	if err = db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, key).Error; err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{}, state.Payload...)
	reuseToggle(t, f, actor, slot.ID, false)
	if closed, e := f.s.p4Commerce(bound, f.id, query, 0, LocalPlayNow(bound)); e != nil || len(closed) != 1 || len(closed[0].(*pb.SCShopProductList).GetShopItemList()) != 0 {
		t.Fatal("disabled scheduled shop remained open", e)
	}
	reuseToggle(t, f, actor, slot.ID, true)
	if err = db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, key).Error; err != nil || string(payload) != string(state.Payload) {
		t.Fatal("shop stop/start reset native limits", err)
	}
	f.restart(t)
	again, err := f.s.p4Commerce(bound, f.id, buy, 93001, LocalPlayNow(bound))
	if err != nil || !proto.Equal(again[0], paid[0]) {
		t.Fatal("cold original shop receipt changed", err)
	}
	current, err := f.s.AdminOrdinaryShopSource(db.GetIns().WithContext(f.ctx), "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	currentRaw, _ := json.Marshal(current)
	if string(currentRaw) != string(raw) {
		t.Fatal("scheduling rewrote the global native source")
	}
}

func TestAdminScheduledVIPUsesNativeGateAndEligibility(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializePayments(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	product := db.VIPShopProduct{ID: "schedule-vip-native", MinVIP: 1, Cost: 200, Reward: "coin,7", Enabled: true, Source: "finite original VIP consumer fixture"}
	if err := db.GetIns().Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminShopAvailability{}).Where("shop_key=?", "vip").Update("unlimited", false).Error; err != nil {
		t.Fatal(err)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var definition model.AdminActivityDefinitionDTO
	for _, d := range definitions {
		if d.ID == "shop_window_vip" {
			definition = d
		}
	}
	if !definition.Enabled || !definition.Availability.Usable {
		t.Fatal("real VIP gate missing from schedule library", definition)
	}
	slot := reusePublish(t, f, actor, definition, 1, 1, "calendar")
	if !strings.HasPrefix(slot.ConsumerID, "shop-gate:") {
		t.Fatal("special shop created a fake P4 consumer", slot.ConsumerID)
	}
	bound := serverClockBindTest(t, f, f.id)
	output, err := f.s.vipShop(bound, f.id, &pb.CSShopProductList{ShopType: proto.Int32(2)}, LocalPlayNow(bound))
	if err != nil {
		t.Fatal("original VIP list behind schedule gate", err)
	}
	found := false
	for _, item := range output[0].(*pb.SCShopProductList).GetShopItemList() {
		if item.GetId() == vipShopSKU(product) {
			found = true
			if item.GetCanBuy() != 0 {
				t.Fatal("schedule bypassed original paid/VIP eligibility")
			}
		}
	}
	if !found {
		t.Fatal("native VIP product missing")
	}
	if _, err = f.s.vipShop(bound, f.id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String(vipShopSKU(product)), BuyCount: proto.Int32(1)}, LocalPlayNow(bound)); err == nil {
		t.Fatal("schedule gave an unpaid role a VIP purchase")
	}
	reuseToggle(t, f, actor, slot.ID, false)
	if _, err = f.s.vipShop(bound, f.id, &pb.CSShopProductList{ShopType: proto.Int32(2)}, LocalPlayNow(bound)); err == nil {
		t.Fatal("disabled VIP gate remained open")
	}
	if got := p1Count(t, &db.P4CommerceRule{}, "id LIKE ?", "shop_schedule_vip_%"); got != 0 {
		t.Fatal("special shop cloned a P4 sale", got)
	}
	var current db.VIPShopProduct
	if err = db.GetIns().First(&current, "id=?", product.ID).Error; err != nil || current != product {
		t.Fatal("gate rewrote native VIP terms", current, err)
	}
}

// Readiness is separate from the operator's explicit template switch.
func TestAdminScheduledShopReadinessAndExplicitStop(t *testing.T) {
	f, actor := reuseFixture(t, false)
	// This readiness test explicitly starts with its source closed, independently of shared P4 operating prerequisites.
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "popularity").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminShopAvailability{}).Where("shop_key=?", "popularity").Update("unlimited", false).Error; err != nil {
		t.Fatal(err)
	}
	read := func() model.AdminActivityDefinitionDTO {
		t.Helper()
		defs, err := f.s.AdminActivityDefinitions(f.ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		for _, def := range defs {
			if def.ID == "shop_window_popularity" {
				return def
			}
		}
		t.Fatal("auto shop template missing")
		return model.AdminActivityDefinitionDTO{}
	}
	notReady := read()
	if !notReady.Enabled || notReady.Availability.Usable {
		t.Fatal("source readiness was stored as an operator stop", notReady)
	}
	// Simulate the auto-disabled row written by the earlier build; no operator save exists.
	if err := db.GetIns().Model(&db.AdminActivityDefinition{}).Where("id=?", notReady.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "popularity").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	ready := read()
	if !ready.Enabled || !ready.Availability.Usable {
		t.Fatal("ready shop did not enter the real schedule library", ready)
	}
	stopped, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, ready.ID, model.AdminActivityDefinitionSaveRequest{Name: ready.Name, Form: ready.Form, DefaultDays: ready.DefaultDays, ClaimDays: ready.ClaimDays, Enabled: false, Version: ready.Version})
	if err != nil || stopped.Enabled {
		t.Fatal("operator stop failed", err)
	}
	for _, enabled := range []bool{false, true} {
		if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "popularity").Update("enabled", enabled).Error; err != nil {
			t.Fatal(err)
		}
		current := read()
		if current.Enabled {
			t.Fatal("refresh overrode the operator's explicit stop")
		}
	}
}

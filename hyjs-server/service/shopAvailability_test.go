package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminShopUnlimitedFrozenCatalogueAndOriginalReceipt edits live operating
// hours around a native purchase while retaining its historical quote and quota.
func TestAdminShopUnlimitedFrozenCatalogueAndOriginalReceipt(t *testing.T) {
	f, actor := adminModesFixture(t)
	p4Fund(t, f)
	// 本例只切换营业时间，先准备独立测试角色的试炼购买资格。
	if err := db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P7RushState{PlayerID: f.id, RecordStars: 100}).Error; err != nil {
		t.Fatal(err)
	}
	var source db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").Order("start_at DESC,id DESC").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	// Freeze one explicit original consumer. Other enabled fixture roots and
	// the live-editable default are independent contracts, not this historical quote.
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=? AND id<>?", "point", source.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	var accepted p4CommerceRules
	if err := json.Unmarshal(source.Rules, &accepted); err != nil {
		t.Fatal(err)
	}
	accepted.LiveEditable = false
	if err := db.GetIns().Model(&source).Update("rules", adminJSON(accepted)).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.GetIns().Model(&source).Updates(map[string]any{"enabled": true, "start_at": now.AddDate(0, 0, -3).UnixMilli(), "end_at": now.AddDate(0, 0, -2).UnixMilli(), "claim_end_at": now.AddDate(0, 0, -2).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	var hours []db.AdminShopAvailability
	if err := db.GetIns().Find(&hours).Error; err != nil || len(hours) != len(AdminShopDefinitions()) {
		t.Fatal("all shops hours migration", err)
	}
	for _, row := range hours {
		if !row.Unlimited {
			t.Fatal("initial switch not enabled", row.ShopKey)
		}
	}
	query := &pb.CSShopProductList{ShopType: proto.Int32(5)}
	listing, err := f.s.p4Commerce(f.ctx, f.id, query, 0, now)
	if err != nil || len(listing[0].(*pb.SCShopProductList).GetShopItemList()) == 0 {
		t.Fatal("expired native source did not open with unlimited hours", err)
	}
	product := listing[0].(*pb.SCShopProductList).GetShopItemList()[0]
	buy := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: product.Id, BuyCount: proto.Int32(1)}
	paid, err := f.s.p4Commerce(f.ctx, f.id, buy, 777901, now)
	if err != nil {
		t.Fatal("original purchase", err)
	}
	var frozen db.P4CommerceRule
	if err := db.GetIns().First(&frozen, "id=?", source.ID).Error; err != nil || len(frozen.Frozen) == 0 {
		t.Fatal("native quote not frozen", err)
	}
	var progress db.P4CommerceState
	if err := db.GetIns().First(&progress, "player_id=? AND rule_id=?", f.id, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	originalState := append([]byte{}, progress.Payload...)
	view, err := f.s.AdminShop(f.ctx, "point")
	if err != nil {
		t.Fatal(err)
	}
	settings := view.Settings
	settings.Unlimited = false
	settings.StartAt = now.AddDate(0, 0, 1).UnixMilli()
	settings.EndAt = now.AddDate(0, 0, 2).UnixMilli()
	settings.ClaimEndAt = settings.EndAt
	limited, err := f.s.AdminSaveShopSettings(f.ctx, actor, "point", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: view.Version})
	if err != nil || limited.Settings.Unlimited {
		t.Fatal("live shop hours cannot change", err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, buy, 777902, now); !errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
		t.Fatal("outside limited hours charged", err)
	}
	again, err := f.s.p4Commerce(f.ctx, f.id, buy, 777901, now)
	if err != nil || !proto.Equal(again[0], paid[0]) {
		t.Fatal("hours blocked or rewrote original receipt", err)
	}
	if err = f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.AdminShop(f.ctx, "point")
	if err != nil || current.Settings.Unlimited {
		t.Fatal("restart reset operator switch", err)
	}
	settings = current.Settings
	settings.Unlimited = true
	reopened, err := f.s.AdminSaveShopSettings(f.ctx, actor, "point", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: current.Version})
	if err != nil || !reopened.Settings.Unlimited || reopened.Settings.PeriodID != source.ID {
		t.Fatal("hours created new stock identity", err)
	}
	var after db.P4CommerceRule
	if err = db.GetIns().First(&after, "id=?", source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.Frozen, frozen.Frozen) {
		t.Fatal("live hours rewrote the historical accepted quote")
	}
	// The live-shop contract adopted in d38180b permits explicitly saved dates
	// and marks mutable terms without resetting the historical purchase identity.
	var (
		beforeRules map[string]any
		afterRules  map[string]any
	)
	if err := json.Unmarshal(frozen.Rules, &beforeRules); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after.Rules, &afterRules); err != nil {
		t.Fatal(err)
	}
	beforeRules["liveEditable"] = true
	if !reflect.DeepEqual(beforeRules, afterRules) {
		t.Fatal("hours changed product terms beyond the live-editing marker")
	}
	if after.StartAt != settings.StartAt || after.EndAt != settings.EndAt || after.ClaimEndAt != settings.ClaimEndAt {
		t.Fatal("live source ignored explicitly saved operating dates")
	}
	if again, err := f.s.p4Commerce(f.ctx, f.id, buy, 777901, now); err != nil || !proto.Equal(again[0], paid[0]) {
		t.Fatal("live hours rewrote the original purchase receipt", err)
	}
	if err = db.GetIns().First(&progress, "player_id=? AND rule_id=?", f.id, source.ID).Error; err != nil || !bytes.Equal(progress.Payload, originalState) {
		t.Fatal("hours reset native quota", err)
	}
}

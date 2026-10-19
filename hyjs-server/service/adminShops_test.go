package service

import (
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strings"
	"testing"
	"time"
)

// TestAdminShopsMutations covers distinct storage adapters and immediately editable catalogues.
func TestAdminShopsMutations(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePayments(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.initializeP9Market(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P13UnionFightShopSource{}); err != nil {
		t.Fatal(err)
	}
	shop, err := f.s.AdminShop(f.ctx, "point")
	if err != nil || len(shop.Items) == 0 {
		t.Fatal("real period", err)
	}
	item := shop.Items[0]
	item.Cost.Amount += 1
	changed, err := f.s.AdminSaveShopProduct(f.ctx, actor, "point", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: shop.Version}, false)
	if err != nil || changed.Version == shop.Version {
		t.Fatal("save period", err)
	}
	if _, err = f.s.AdminSaveShopProduct(f.ctx, actor, "point", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: shop.Version}, false); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale shop editor", err)
	}
	var period db.P4CommerceRule
	if err = db.GetIns().First(&period, "id=?", changed.Settings.PeriodID).Error; err != nil {
		t.Fatal(err)
	}
	if _, rules, e := f.s.p4CommerceSnapshot(period); e != nil || int64(adminTypedResourceOne(rules.Products[0].Cost).Amount) != item.Cost.Amount {
		t.Fatal("original consumer missed typed changes", e)
	}
	// A frozen historical quote is a declared prerequisite, never a fabricated purchase.
	if err = db.GetIns().Model(&period).Update("frozen", []byte(adminJSON(period))).Error; err != nil {
		t.Fatal(err)
	}
	frozen, err := f.s.AdminShop(f.ctx, "point")
	if err != nil || frozen.Settings.Frozen {
		t.Fatal("shop must remain editable after prior purchases", err)
	}
	item.Cost.Amount++
	changed, err = f.s.AdminSaveShopProduct(f.ctx, actor, "point", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: frozen.Version}, false)
	if err != nil {
		t.Fatal("live catalogue edit", err)
	}
	settings := changed.Settings
	settings.StartAt += 1000
	settings.EndAt += 1000
	settings.ClaimEndAt += 1000
	next, err := f.s.AdminSaveShopSettings(f.ctx, actor, "point", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: changed.Version, NewPeriod: true})
	if err != nil || next.Settings.Frozen || next.Settings.PeriodID != changed.Settings.PeriodID {
		t.Fatal("settings edit must retain existing quota identity", err)
	}
	vip, err := f.s.AdminShop(f.ctx, "vip")
	if err != nil {
		t.Fatal(err)
	}
	v := model.AdminShopProduct{Rewards: []model.AdminResourceAmount{{ResourceID: "item_001", Amount: 1}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, Source: "isolated operator fixture", MinVip: 1, Enabled: false}
	vip, err = f.s.AdminSaveShopProduct(f.ctx, actor, "vip", "", model.AdminShopProductSaveRequest{AdminShopProduct: v, Version: vip.Version}, true)
	if err != nil {
		t.Fatal("VIP adapter", err)
	}
	pet, err := f.s.AdminShop(f.ctx, "pet-market")
	if err != nil {
		t.Fatal(err)
	}
	v = pet.Items[0]
	v.Cost.Amount++
	pet, err = f.s.AdminSaveShopProduct(f.ctx, actor, "pet-market", v.ID, model.AdminShopProductSaveRequest{AdminShopProduct: v, Version: pet.Version}, false)
	if err != nil {
		t.Fatal("pet adapter", err)
	}
	uf, err := f.s.AdminShop(f.ctx, "union-fight")
	if err != nil || len(uf.Resources) == 0 {
		t.Fatal("UF original resources", err)
	}
	for _, resource := range uf.Resources {
		if resource.Name == "" || strings.HasPrefix(resource.Name, "name_") || resource.IconURL == "" {
			t.Fatalf("unresolved original union resource: %+v", resource)
		}
	}
	v = model.AdminShopProduct{Rewards: []model.AdminResourceAmount{{ResourceID: uf.Resources[0].ID, Amount: 1}}, Cost: model.AdminResourceAmount{ResourceID: "unionFightToken", Amount: 10}, Day: time.Now().Format("2006-01-02"), Stock: 1, RefreshMillis: 1000, Source: "isolated operator fixture", Enabled: false}
	uf, err = f.s.AdminSaveShopProduct(f.ctx, actor, "union-fight", "", model.AdminShopProductSaveRequest{AdminShopProduct: v, Version: uf.Version}, true)
	if err != nil || len(uf.Items) != 1 {
		t.Fatal("UF adapter", err)
	}
	if _, err = f.s.AdminDeleteShopProduct(f.ctx, actor, "union-fight", uf.Items[0].ID, uf.Version); err != nil {
		t.Fatal("UF retain history and disable", err)
	}
	f.restart(t)
	after, err := f.s.AdminShop(f.ctx, "point")
	if err != nil || after.Settings.PeriodID != next.Settings.PeriodID {
		t.Fatal("cold persisted shop", err)
	}
}

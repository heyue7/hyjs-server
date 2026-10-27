package service

import (
	"testing"

	"hyjs-server/model"
)

func TestTrialShopAdminRetainsStarsForLegacyEdit(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	shop, err := f.s.AdminShop(f.ctx, "point")
	if err != nil || len(shop.Items) == 0 {
		t.Fatal("point catalogue", err)
	}
	item := shop.Items[0]
	stars := int64(135)
	item.MinTrialStars = &stars
	shop, err = f.s.AdminSaveShopProduct(f.ctx, actor, "point", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: shop.Version}, false)
	if err != nil {
		t.Fatal(err)
	}
	item = shop.Items[0]
	if item.MinTrialStars == nil || *item.MinTrialStars != stars {
		t.Fatal("star requirement not returned")
	}
	// An older admin client omits the new field while editing another property.
	item.MinTrialStars = nil
	item.Cost.Amount++
	shop, err = f.s.AdminSaveShopProduct(f.ctx, actor, "point", item.ID, model.AdminShopProductSaveRequest{AdminShopProduct: item, Version: shop.Version}, false)
	if err != nil {
		t.Fatal(err)
	}
	if shop.Items[0].MinTrialStars == nil || *shop.Items[0].MinTrialStars != stars {
		t.Fatal("legacy edit removed star requirement")
	}
}

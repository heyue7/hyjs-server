package service

import (
	"bytes"
	"encoding/json"
	"testing"

	"hyjs-server/db"
	"hyjs-server/model"
)

func TestAdminOrdinaryShop55RetiredStaminaAndLiveSwitch(t *testing.T) {
	f, actor := adminModesFixture(t)
	var source db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "ordinary").Order("start_at DESC,id DESC").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(source.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	// A catalogue from before the potion-only change remains a valid historical source.
	retired := p4BaseProduct("retired-stamina55", "stamina,25", "diamond,20", 200, 1)
	rules.Products = append(rules.Products, retired)
	source.Rules = adminJSON(rules)
	if err := db.GetIns().Save(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.ShopProduct{ID: "legacy-stamina55", ResourceID: "stamina", Amount: 25, Cost: "diamond,20", MinLevel: 1, DailyLimit: 20}).Error; err != nil {
		t.Fatal(err)
	}
	page, err := f.s.AdminShop(f.ctx, "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	var potion *model.AdminShopProduct
	for i := range page.Items {
		item := &page.Items[i]
		for _, reward := range item.Rewards {
			if reward.ResourceID == "stamina" {
				t.Fatal("retired direct-stamina product still offers an admin switch")
			}
			if reward.ResourceID == "item_002" && len(item.ID) > 7 && item.ID[:7] == "period:" {
				potion = item
			}
		}
	}
	if potion == nil {
		t.Fatal("active potion product missing")
	}
	potion.Enabled = !potion.Enabled
	changed, err := f.s.AdminSaveShopProduct(f.ctx, actor, "ordinary", potion.ID, model.AdminShopProductSaveRequest{AdminShopProduct: *potion, Version: page.Version}, false)
	if err != nil {
		t.Fatal("normal product switch", err)
	}
	found := false
	for _, item := range changed.Items {
		if item.ID == potion.ID {
			found = item.Enabled == potion.Enabled
		}
	}
	if !found {
		t.Fatal("saved switch not reflected")
	}
	var after db.P4CommerceRule
	if err := db.GetIns().First(&after, "id=?", source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.Frozen, source.Frozen) {
		t.Fatal("historical frozen source changed")
	}
	if err := json.Unmarshal(after.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	for _, product := range rules.Products {
		if product.ID == retired.ID {
			if product.Rewards != retired.Rewards || product.isEnabled() != retired.isEnabled() {
				t.Fatal("retired stored terms changed")
			}
			return
		}
	}
	t.Fatal("retired history was deleted")
}

package service

import (
	"encoding/json"
	"hyjs-server/db"
	"testing"
	"time"
)

// Finite, explicitly published unit-test sources. Real trial End must still
// create every earned star and point; no outcome or event fact is injected.
func p7StarActivityOperations75(t *testing.T, f *p1Fixture) {
	t.Helper()
	if err := f.s.seedP7StarActivity(db.GetIns(), time.Now(), true); err != nil {
		t.Fatal(err)
	}
}

func p7PointShopOperations75(t *testing.T, f *p1Fixture) {
	t.Helper()
	now := time.Now()
	rules := p4CommerceRules{Products: []p4Product{
		p4BaseProduct("p4Point_1", "item_021,1", "point,100", 1, 1),
		p4BaseProduct("p4Point_2", "frag_equip_037,10", "item_021,10", 2, 1),
	}}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	row := db.P4CommerceRule{ID: "p7-point-fixture75", Kind: "point", ShopType: 5, Enabled: true, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(24 * time.Hour).UnixMilli(), ClaimEndAt: now.Add(24 * time.Hour).UnixMilli(), Rules: raw, Source: "explicit finite P7 earned-point consumer fixture; ten original fragments, no direct complete red gear"}
	if err = db.GetIns().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}

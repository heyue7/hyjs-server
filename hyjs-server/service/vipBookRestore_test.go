package service

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func vipBookRestoreFixture(t *testing.T) (*GameService, context.Context, *model.AdminPrincipal, db.VIPShopProduct) {
	t.Helper()
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "vip-restore.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool, _ := db.GetIns().DB(); pool.Close() })
	if err := db.GetIns().AutoMigrate(&db.VIPShopProduct{}, &db.AdminUser{}, &db.AdminAudit{}, &db.ClientUpdatePolicy{}, &db.MembershipClaim{}); err != nil {
		t.Fatal(err)
	}
	u := db.AdminUser{Username: "isolated-vip-restorer", Role: constUtil.AdminRoleAdmin, Enabled: true, PasswordHash: "fixture"}
	product := db.VIPShopProduct{ID: constUtil.P3VIP10GrowthProduct, MinVIP: 10, Cost: 1100, CostResource: "diamond", Reward: "item_044,1", Source: constUtil.P3VIP10GrowthSource}
	for _, row := range []any{&u, &product, &db.ClientUpdatePolicy{ID: 1, Maintenance: true}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &GameService{Tables: map[string][]map[string]string{"ItemUse": {{"itemId": "item_044", "category": "2", "odds": "100", "result": constUtil.VIPBookRestoreBoxResult}}}}
	return s, ctx, &model.AdminPrincipal{UserID: u.ID, Username: u.Username, Role: u.Role}, product
}

func TestVIPBookRestorePreservesTermsHistoryAndLaterStop(t *testing.T) {
	s, ctx, actor, before := vipBookRestoreFixture(t)
	claim := db.MembershipClaim{PlayerID: 42, Kind: "vip-shop:" + before.ID, Period: "2026-10-26", Reward: before.Reward, At: 1}
	if err := db.GetIns().Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreVIP10BookChannel(ctx, actor); err != nil {
		t.Fatal(err)
	}
	var after db.VIPShopProduct
	if err := db.GetIns().First(&after, "id=?", before.ID).Error; err != nil {
		t.Fatal(err)
	}
	want := before
	want.Enabled = true
	if !reflect.DeepEqual(want, after) {
		t.Fatalf("terms changed: %+v", after)
	}
	if err := db.GetIns().Model(&after).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreVIP10BookChannel(ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&after, "id=?", before.ID).Error; err != nil || after.Enabled {
		t.Fatal("retry undid manual stop", err)
	}
	var saved db.MembershipClaim
	if err := db.GetIns().First(&saved, "player_id=?", 42).Error; err != nil || !reflect.DeepEqual(claim, saved) {
		t.Fatal("history changed", saved, err)
	}
	if n := p1Count(t, &db.AdminAudit{}, "action=?", constUtil.VIPBookRestoreAction); n != 1 {
		t.Fatal("duplicate audit", n)
	}
}

func TestVIPBookRestoreGuardsAndAtomicAudit(t *testing.T) {
	for _, scenario := range []string{"live", "price", "reward", "limit", "source", "operator", "parcel", "audit-failure"} {
		t.Run(scenario, func(t *testing.T) {
			s, ctx, actor, product := vipBookRestoreFixture(t)
			var err error
			switch scenario {
			case "live":
				err = db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", false).Error
			case "price":
				err = db.GetIns().Model(&product).Update("cost", 1200).Error
			case "reward":
				err = db.GetIns().Model(&product).Update("reward", "item_064,1").Error
			case "limit":
				err = db.GetIns().Model(&product).Update("total_limit", 1).Error
			case "source":
				err = db.GetIns().Model(&product).Update("source", "operator edited").Error
			case "operator":
				err = db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleOperator).Error
			case "parcel":
				s.Tables["ItemUse"][0]["result"] = "item_064,1"
			case "audit-failure":
				err = db.GetIns().Exec("CREATE TRIGGER reject_vip_audit BEFORE INSERT ON admin_audits BEGIN SELECT RAISE(ABORT, 'isolated audit failure'); END").Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = s.RestoreVIP10BookChannel(ctx, actor); err == nil {
				t.Fatal("guard allowed", scenario)
			}
			if err = db.GetIns().First(&product, "id=?", product.ID).Error; err != nil || product.Enabled {
				t.Fatal("failed restore changed enabled", err)
			}
		})
	}
}

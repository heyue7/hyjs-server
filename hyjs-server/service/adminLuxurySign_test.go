package service

import (
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

// TestAdminLuxurySignConfigurationBoundary verifies fixed-source editing and original consumer windows.
func TestAdminLuxurySignConfigurationBoundary(t *testing.T) {
	f, actor := adminModesFixture(t)
	tx := db.GetIns()
	if err := tx.AutoMigrate(&db.ActivityRules{}, &db.ActivityReward{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("activity_id=?", constUtil.P3LuxurySignActivityID).Delete(&db.ActivityReward{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("id=?", constUtil.P3LuxurySignActivityID).Delete(&db.ActivityRules{}).Error; err != nil {
		t.Fatal(err)
	}
	initial, err := f.s.AdminLuxurySignRules(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Enabled || len(initial.Rewards) != 0 || initial.RechargeCents != 600 {
		t.Fatal("missing source fabricated defaults", initial)
	}
	viewer := db.AdminUser{Username: "luxury64-viewer", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err := tx.Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	readOnly := &model.AdminPrincipal{UserID: viewer.ID, Username: viewer.Username, Role: viewer.Role}
	if _, err := f.s.AdminLuxurySignRules(f.ctx, readOnly); err != nil {
		t.Fatal("viewer read rejected", err)
	}
	if _, err := f.s.AdminSaveLuxurySignRules(f.ctx, readOnly, model.AdminLuxurySignRulesSaveRequest{Version: initial.Version}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer write accepted", err)
	}
	req := model.AdminLuxurySignRulesSaveRequest{Version: initial.Version, Enabled: true}
	if _, err = f.s.AdminSaveLuxurySignRules(f.ctx, actor, req); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("empty enabled rewards accepted", err)
	}
	req.Rewards = []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 2}}
	saved, err := f.s.AdminSaveLuxurySignRules(f.ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if saved.StartAt != 1 || saved.EndAt != constUtil.AdminPermanentActivityEndAt {
		t.Fatal("permanent window missing", saved)
	}
	if _, err = f.s.AdminSaveLuxurySignRules(f.ctx, actor, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version accepted", err)
	}
	reward, err := luxurySignReward(tx, time.Now())
	if err != nil || reward != "diamond,2" {
		t.Fatal("original consumer cannot read saved rule", reward, err)
	}
	future := time.Now().Add(time.Hour).UnixMilli()
	if err = tx.Model(&db.ActivityRules{}).Where("id=?", constUtil.P3LuxurySignActivityID).Updates(map[string]any{"start_at": future, "end_at": future + 3600000}).Error; err != nil {
		t.Fatal(err)
	}
	current, err := f.s.AdminLuxurySignRules(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	current, err = f.s.AdminSaveLuxurySignRules(f.ctx, actor, model.AdminLuxurySignRulesSaveRequest{Version: current.Version, Enabled: true, Rewards: saved.Rewards})
	if err != nil || current.StartAt != future {
		t.Fatal("future window overwritten", current, err)
	}
	if err = tx.Model(&db.ActivityRules{}).Where("id=?", constUtil.P3LuxurySignActivityID).Updates(map[string]any{"start_at": 1, "end_at": 2}).Error; err != nil {
		t.Fatal(err)
	}
	current, err = f.s.AdminLuxurySignRules(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	current, err = f.s.AdminSaveLuxurySignRules(f.ctx, actor, model.AdminLuxurySignRulesSaveRequest{Version: current.Version, Enabled: true, Rewards: saved.Rewards})
	if err != nil || current.EndAt != constUtil.AdminPermanentActivityEndAt {
		t.Fatal("expired explicit reopen failed", current, err)
	}
	if err = tx.Create(&db.ActivityReward{ActivityID: constUtil.P3LuxurySignActivityID, RewardType: constUtil.P3LuxurySignRewardType, Threshold: 1, Reward: "coin,1"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminLuxurySignRules(f.ctx, actor); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatal("ambiguous daily sources accepted", err)
	}
}

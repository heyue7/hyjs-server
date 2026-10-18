package service

import (
	"errors"
	"strings"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminPoolArchiveKeepsHistory exercises the browser's closed copied-pool DELETE path.
func TestAdminPoolArchiveKeepsHistory(t *testing.T) {
	f, operator := adminModesFixture(t)
	if err := db.GetIns().AutoMigrate(&db.TurntableRules{}, &db.TurntableState{}); err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("turntable")
	form.Options.ScorePerDraw = 1
	form.Options.AdvancePlaces = 1
	form.Options.TopPrize = model.AdminResourceAmount{ResourceID: "diamond", Amount: 1}
	form.Options.AdvancePrize = model.AdminResourceAmount{ResourceID: "diamond", Amount: 1}
	for i := int64(0); i < 9; i++ {
		form.Prizes = append(form.Prizes, model.AdminActivityPrize{ID: i, ResourceID: "coin", Amount: 1, Weight: 1})
	}
	created, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: false})
	if err != nil {
		t.Fatal("closed pool create", err)
	}
	if created.Enabled || created.Frozen {
		t.Fatal("new closed pool state")
	}
	if err := f.s.AdminDeleteTypedActivity(f.ctx, operator, created.ID, created.Version); err != nil {
		t.Fatal("closed pool archive", err)
	}
	_, key, _ := strings.Cut(created.ID, ":")
	var pool db.GameplayPool
	if err := db.GetIns().First(&pool, "id=?", key).Error; err != nil || pool.Enabled {
		t.Fatal("archived pool disappeared or enabled", err)
	}
	if count := p1Count(t, &db.GameplayPoolReward{}, "pool_id=?", key); count != 9 {
		t.Fatal("archive deleted original prize slots", count)
	}
	var meta db.AdminActivityPresentation
	if err := db.GetIns().First(&meta, "activity_id=?", created.ID).Error; err != nil || !meta.Archived {
		t.Fatal("archive metadata not committed", err)
	}
	if err := f.s.AdminDeleteTypedActivity(f.ctx, operator, created.ID, created.Version); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale repeated archive should conflict, not return database error", err)
	}
}

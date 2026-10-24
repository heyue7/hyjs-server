package service

import (
	"context"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

func TestActivitySchedule63ArchiveRestore(t *testing.T) {
	f, operator := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("discount-shop")
	form.Products = []model.AdminActivityProduct{{ID: "archive_product", Name: "道具", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 2}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 2, TotalLimit: 2, Stock: -1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	def, err := f.s.AdminSaveActivityDefinition(f.ctx, operator, "", model.AdminActivityDefinitionSaveRequest{Name: "归档恢复检查", Form: form, DefaultDays: 2, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, operator, 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	saved, err := f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: def.ID, TemplateVersion: def.Version, StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Enabled: false}}})
	if err != nil {
		t.Fatal(err)
	}
	slot := saved.Items[0]
	archived, err := f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: saved.Version, Items: []model.AdminActivityOccurrenceDTO{}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.s.AdminArchivedActivitySchedules(f.ctx, operator, 1)
	if err != nil || len(rows) != 1 || rows[0].ID != slot.ID {
		t.Fatalf("archive=%v %v", rows, err)
	}
	if _, err := f.s.AdminRestoreActivitySchedule(f.ctx, operator, 1, slot.ID, model.AdminActivityScheduleRestoreRequest{Version: saved.Version}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale restore=%v", err)
	}
	restored, err := f.s.AdminRestoreActivitySchedule(f.ctx, operator, 1, slot.ID, model.AdminActivityScheduleRestoreRequest{Version: archived.Version})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Items) != 1 || restored.Items[0].ID != slot.ID || restored.Items[0].Enabled || restored.Items[0].ConsumerID != slot.ConsumerID {
		t.Fatal("restore changed history or enabled consumer")
	}
	var consumer db.P4CommerceRule
	if err := db.GetIns().First(&consumer, "id=?", slot.ConsumerID[len("commerce:"):]).Error; err != nil || consumer.Enabled {
		t.Fatal("restored consumer reopened", err)
	}
	rows, err = f.s.AdminArchivedActivitySchedules(f.ctx, operator, 1)
	if err != nil || len(rows) != 0 {
		t.Fatal("restored row still archived")
	}
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminActivityScheduleRuntime covers real typed config -> realm publication -> original shop purchase,
// foreign-realm denial, concurrent version protection and cold immutable receipts in one finite fixture.
func TestAdminActivityScheduleRuntime(t *testing.T) {
	f, operator := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.AdminGameServer{ID: 2, Name: "隔离二区", Enabled: true, OpenAt: time.Now().UnixMilli(), Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	form := adminModeForm("discount-shop")
	form.Products = []model.AdminActivityProduct{{ID: "calendar_product", Name: "原兑换道具", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 2}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 2, TotalLimit: 2, Stock: -1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	initialConsumers := int64(0)
	db.GetIns().Model(&db.P4CommerceRule{}).Count(&initialConsumers)
	template, err := f.s.AdminSaveActivityDefinition(f.ctx, operator, "", model.AdminActivityDefinitionSaveRequest{Name: "全局复用礼包", Form: form, DefaultDays: 2, ClaimDays: 0, Enabled: true})
	if err != nil {
		t.Fatal("global template", err)
	}
	afterConsumers := int64(0)
	db.GetIns().Model(&db.P4CommerceRule{}).Count(&afterConsumers)
	if initialConsumers != afterConsumers {
		t.Fatal("global draft leaked a gameplay consumer", initialConsumers, afterConsumers)
	}
	if !template.Availability.Usable {
		t.Fatal("valid typed template unavailable")
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, operator, 1)
	if err != nil {
		t.Fatal(err)
	}
	pending := model.AdminActivityOccurrenceDTO{TemplateID: template.ID, TemplateVersion: template.Version, StartAt: now - 1000, EndAt: now + 2*86400000, ClaimEndAt: now + 2*86400000, Enabled: true}
	saved, err := f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{pending}})
	if err != nil || len(saved.Items) != 1 {
		t.Fatal("realm publish", saved, err)
	}
	slot := saved.Items[0]
	if slot.Frozen || slot.ID == "" || !strings.Contains(slot.ConsumerID, "_schedule_") {
		t.Fatal("published consumer binding", slot)
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: saved.Items}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale whole-calendar write", err)
	}
	viewer := db.AdminUser{Username: "calendar-viewer", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err = db.GetIns().Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	read := &model.AdminPrincipal{UserID: viewer.ID, Username: viewer.Username, Role: viewer.Role}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, read, 1, model.AdminActivityScheduleSaveRequest{Version: saved.Version, Items: saved.Items}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer wrote calendar", err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(20)
		p.Diamond = proto.Int64(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, key, _ := strings.Cut(slot.ConsumerID, ":")
	// Create a separate deployment using immutable configuration, without copying atomic runtime state.
	foreign := &GameService{ServerID: 2, Tables: f.s.Tables, P1Ready: f.s.P1Ready, P4Ready: f.s.P4Ready}
	foreignCtx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, foreign)
	if _, err = foreign.p4Commerce(foreignCtx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, time.Now()); err == nil {
		t.Fatal("foreign realm guessed and loaded private occurrence")
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, time.Now()); err != nil {
		t.Fatal("original info could not use calendar publication", err)
	}
	buy := &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(10)}}, GetResources: []*pb.BaseResource{{Id: proto.String("item_005"), Num: proto.Int32(2)}}}
	reply, err := f.s.p4Commerce(f.ctx, f.id, buy, 996210, time.Now())
	if err != nil {
		t.Fatal("original purchase", err)
	}
	paidP, paidG := f.profile(t)
	if paidP.GetDiamond() != 90 || simpleOwned(paidP, paidG, "item_005") != 2 {
		t.Fatal("actual cost or grant", paidP.GetDiamond(), simpleOwned(paidP, paidG, "item_005"))
	}
	frozen, err := f.s.AdminActivitySchedule(f.ctx, operator, 1)
	if err != nil || !frozen.Items[0].Frozen {
		t.Fatal("consumer did not freeze", err)
	}
	moved := append([]model.AdminActivityOccurrenceDTO{}, frozen.Items...)
	moved[0].EndAt += 86400000
	moved[0].ClaimEndAt += 86400000
	preview, err := f.s.AdminPreviewActivitySchedule(f.ctx, operator, 1, model.AdminActivitySchedulePreviewRequest{Version: frozen.Version, Items: moved})
	if err != nil || preview.Valid {
		t.Fatal("frozen move preview", preview, err)
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: frozen.Version, Items: moved}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("frozen times overwritten", err)
	}
	form.Products[0].Rewards[0].Amount = 7
	updated, err := f.s.AdminSaveActivityDefinition(f.ctx, operator, template.ID, model.AdminActivityDefinitionSaveRequest{Name: template.Name, Form: form, DefaultDays: 2, Enabled: true, Version: template.Version})
	if err != nil || updated.Version == template.Version {
		t.Fatal("global future edit", err)
	}
	var occurrence db.AdminActivityOccurrence
	if err = db.GetIns().First(&occurrence, "id=?", slot.ID).Error; err != nil {
		t.Fatal(err)
	}
	var retained model.AdminActivityForm
	if json.Unmarshal(occurrence.FormJSON, &retained) != nil || retained.Products[0].Rewards[0].Amount != 2 {
		t.Fatal("global edit rewrote published snapshot")
	}
	// Same-mode no-instance windows are rejected, while different original modes can remain parallel.
	loginForm := adminModeForm("login-rewards")
	loginForm.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 11}}}}
	loginTemplate, err := f.s.AdminSaveActivityDefinition(f.ctx, operator, "", model.AdminActivityDefinitionSaveRequest{Name: "奖历", Form: loginForm, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	overlaps := append([]model.AdminActivityOccurrenceDTO{}, frozen.Items...)
	for i := 0; i < 2; i++ {
		overlaps = append(overlaps, model.AdminActivityOccurrenceDTO{TemplateID: loginTemplate.ID, TemplateVersion: loginTemplate.Version, StartAt: now + 86400000, EndAt: now + 2*86400000, ClaimEndAt: now + 2*86400000, Enabled: true})
	}
	preview, err = f.s.AdminPreviewActivitySchedule(f.ctx, operator, 1, model.AdminActivitySchedulePreviewRequest{Version: frozen.Version, Items: overlaps})
	if err != nil || preview.Valid || len(preview.Conflicts) != 1 {
		t.Fatal("singleton overlap", preview, err)
	}
	pattern, err := f.s.AdminSaveActivityPattern(f.ctx, operator, "", model.AdminActivityPatternSaveRequest{Name: "两天轮换", CycleDays: 2, Entries: []model.AdminActivityPatternEntry{{TemplateID: template.ID, DurationDays: 2}}})
	if err != nil {
		t.Fatal("pattern create", err)
	}
	if _, err = f.s.AdminSaveActivityPattern(f.ctx, operator, pattern.ID, model.AdminActivityPatternSaveRequest{Name: pattern.Name, CycleDays: 3, Entries: pattern.Entries, Version: "stale"}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("pattern stale", err)
	}
	if err = f.s.AdminDeleteActivityPattern(f.ctx, operator, pattern.ID, pattern.Version); err != nil {
		t.Fatal("pattern archive", err)
	}
	// A calendar-aware realm with no current turntable must not silently reopen the legacy public wheel.
	if pool, err := f.s.AdminScheduledTurntablePool(db.GetIns().WithContext(f.ctx), now, "legacy_pool"); err != nil || pool != "" {
		t.Fatal("calendar gap reopened legacy wheel", pool, err)
	}
	if pool, err := foreign.AdminScheduledTurntablePool(db.GetIns().WithContext(foreignCtx), now, "legacy_pool"); err != nil || pool != "legacy_pool" {
		t.Fatal("unmanaged realm compatibility", pool, err)
	}
	// Two whole-calendar publishers cannot overwrite each other from the same version.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: frozen.Version, Items: frozen.Items})
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, constUtil.ErrAdminConfigConflict) {
			conflict++
		} else {
			t.Fatal("concurrent save", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("calendar lost-update guard", success, conflict)
	}
	f.restart(t)
	replay, err := f.s.p4Commerce(f.ctx, f.id, buy, 996210, time.Now())
	if err != nil || len(replay) != len(reply) || !proto.Equal(replay[0], reply[0]) {
		t.Fatal("cold original receipt", err)
	}
	coldP, coldG := f.profile(t)
	if coldP.GetDiamond() != paidP.GetDiamond() || simpleOwned(coldP, coldG, "item_005") != 2 {
		t.Fatal("cold retry granted twice")
	}
	var audits int64
	if err = db.GetIns().Model(&db.AdminAudit{}).Where("action=?", "activity.schedule.save").Count(&audits).Error; err != nil || audits != 2 {
		t.Fatal("atomic successful-only audit", audits, err)
	}
}

// TestAdminActivityScheduleScopeEmpty checks empty publication, retired window guards and per-character opening anchors.
func TestAdminActivityScheduleScopeEmpty(t *testing.T) {
	f, operator := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Keep independent test runs reusable in a single managed MySQL schema.
	if err := db.GetIns().AutoMigrate(&db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminActivityPattern{}, &db.AdminActivityScheduleRevision{}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminActivityPattern{}, &db.AdminActivityScheduleRevision{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	before, err := localPlayActivityVisible(db.GetIns().WithContext(f.ctx), "discountSale_p4_202610")
	if err != nil || !before {
		t.Fatal("unmanaged compatibility", before, err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, operator, 1)
	if err != nil {
		t.Fatal(err)
	}
	if calendar.Managed {
		t.Fatal("unpublished legacy realm reported managed")
	}
	published, err := f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{}})
	if err != nil || !published.Managed || len(published.Items) != 0 {
		t.Fatal("empty publication must switch the realm into managed mode", published, err)
	}
	cold, err := f.s.AdminActivitySchedule(f.ctx, operator, 1)
	if err != nil || !cold.Managed || cold.Version != published.Version {
		t.Fatal("managed state did not survive a fresh calendar read", cold, err)
	}
	for _, key := range []string{"discountSale_p4_202610", "p4:login:202610", "rechargeTotal_p4_202610"} {
		allowed, e := localPlayActivityVisible(db.GetIns().WithContext(f.ctx), key)
		if e != nil || allowed {
			t.Fatal("explicit empty calendar leaked legacy mode", key, e)
		}
	}
	if allowed, err := localPlayActivityAccess(db.GetIns().WithContext(f.ctx), "discountSale_p4_202610", false); err != nil || allowed {
		t.Fatal("direct claim bypassed calendar", err)
	}
	cfg := &db.OpenServerActivityRule{Enabled: true, StartAt: 1, EndAt: 2, RewardEndAt: 3}
	if err = AdminScheduledOpeningRule(db.GetIns().WithContext(f.ctx), cfg, time.Now().UnixMilli()); err != nil || cfg.Enabled || cfg.StartAt != 1 || cfg.EndAt != 2 || cfg.RewardEndAt != 3 {
		t.Fatal("opening gate rewrote original character window", cfg, err)
	}
	// Ordinary shop is independent of activity scheduling.
	if allowed, err := localPlayActivityVisible(db.GetIns().WithContext(f.ctx), "p4:ordinary:202610"); err != nil || !allowed {
		t.Fatal("ordinary shop was hidden", err)
	}
	if key, err := AdminCurrentWelfareRuleKey(db.GetIns().WithContext(f.ctx), "login", time.Now().UnixMilli(), "p4:login:202610"); err != nil || !strings.HasPrefix(key, "inactive_schedule_") {
		t.Fatal("no-instance legacy fallback reopened", key, err)
	}
}

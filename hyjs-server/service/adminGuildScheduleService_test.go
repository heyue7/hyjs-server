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

func TestAdminGuildScheduleAccessRevisionFreeze(t *testing.T) {
	f, _ := p13GuildSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13UnionFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightSchedule{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("preparation_millis", 300000).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.GetIns().Where("guild_id=?", "explicit-frozen-schedule-test").Delete(&db.P13UnionFightEntry{})
	})
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	viewer := adminGuildTestActor(t, constUtil.AdminRoleViewer)
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	future := LocalPlayNow(ctx).In(time.FixedZone("Asia/Shanghai", 8*3600)).AddDate(0, 0, 8).Format("2006-01-02")
	req := model.AdminGuildScheduleSaveRequest{AdminGuildScheduleForm: model.AdminGuildScheduleForm{Day: future, EnrollmentStart: future + " 10:00", EnrollmentEnd: future + " 19:55", FirstStartTime: "20:00", SecondStartTime: "21:00", DurationMinutes: 30, Enabled: true, Version: "future-calendar-v1"}, Reason: "explicit test calendar publication"}
	if _, err := f.s.AdminSaveGuildSchedule(ctx, viewer, req, true); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer can publish", err)
	}
	row, err := f.s.AdminSaveGuildSchedule(ctx, actor, req, true)
	if err != nil || !row.Editable || row.Frozen || row.Legacy || row.FirstEnd != future+" 20:30" {
		t.Fatal("human calendar create", row, err)
	}
	req.RowVersion = row.RowVersion
	req.Version = "future-calendar-v2"
	updated, err := f.s.AdminSaveGuildSchedule(ctx, actor, req, false)
	if err != nil || updated.RowVersion == row.RowVersion {
		t.Fatal("versioned schedule update", err)
	}
	if _, err := f.s.AdminSaveGuildSchedule(ctx, actor, req, false); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale writer accepted", err)
	}
	// A different battle day cannot reuse a registration identity or overlapping window.
	changed := req
	changed.RowVersion = ""
	changed.Day = LocalPlayNow(ctx).In(time.FixedZone("Asia/Shanghai", 8*3600)).AddDate(0, 0, 9).Format("2006-01-02")
	if _, err := f.s.AdminSaveGuildSchedule(ctx, actor, changed, true); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("reused/overlapping registration accepted", err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightEntry{GuildID: "explicit-frozen-schedule-test", Day: row.RegistrationDay, AppliedAt: 1, Deadline: 1, Source: "explicit entry prevents all calendar rewrites"}).Error; err != nil {
		t.Fatal(err)
	}
	req.RowVersion = updated.RowVersion
	req.Enabled = false
	if _, err := f.s.AdminSaveGuildSchedule(ctx, actor, req, false); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("declared schedule was disabled", err)
	}
	listed, err := f.s.AdminGuildSchedules(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range listed.Items {
		if item.Day == future {
			found = true
			if !item.Frozen || item.Editable {
				t.Fatal("locked schedule is editable")
			}
		}
	}
	if !found {
		t.Fatal("schedule missing")
	}
	var audits int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action=? AND target=?", "guild.schedule.save", future).Count(&audits).Error; err != nil || audits != 2 {
		t.Fatal("publication audit/rejected writes", audits, err)
	}
}

func TestAdminGuildCalendarModePendingGuard(t *testing.T) {
	f, _ := p13GuildSetup(t)
	// Mode transition fixture starts with no unfinished enrollment or match.
	for _, table := range []any{&db.P13UnionFightEntry{}, &db.P13UnionFightMatch{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.GetIns().Where("guild_id=?", "pending-mode-guard").Delete(&db.P13UnionFightEntry{})
		db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false)
	})
	if err := f.s.InitializeP13UnionFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	before, err := f.s.AdminGuildSchedules(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.s.AdminSaveGuildCalendar(ctx, actor, model.AdminGuildCalendarSaveRequest{Enabled: true, RowVersion: before.PolicyVersion, Reason: "explicit custom mode"})
	if err != nil || !after.CustomCalendarEnabled {
		t.Fatal("mode not saved", err)
	}
	if _, err := f.s.AdminSaveGuildCalendar(ctx, actor, model.AdminGuildCalendarSaveRequest{Enabled: false, RowVersion: before.PolicyVersion, Reason: "stale toggle"}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("mode stale write accepted", err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightEntry{GuildID: "pending-mode-guard", Day: "2099-01-01", AppliedAt: 1, Deadline: LocalPlayNow(ctx).Add(time.Hour).UnixMilli(), MatchProcessed: false, Source: "explicit unfinished enrollment"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminSaveGuildCalendar(ctx, actor, model.AdminGuildCalendarSaveRequest{Enabled: false, RowVersion: after.PolicyVersion, Reason: "cannot strand pending roster"}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("unprocessed entry lost its calendar", err)
	}
}

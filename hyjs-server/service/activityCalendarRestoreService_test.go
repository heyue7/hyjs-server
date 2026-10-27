package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"reflect"
	"strings"
	"testing"
	"time"
)

func calendarRestoreTransactionFixture(t *testing.T) (*GameService, *model.AdminPrincipal) {
	_, _, _, _, _, _ = openingRestoreFixture(t)
	for _, entity := range []any{&db.AdminUser{}, &db.AdminAudit{}, &db.ServerGameClock{}, &db.ServerGameClockOperation{}, &db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}, &db.AdminActivityScheduleRevision{}, &db.ClientUpdatePolicy{}, &db.PaymentOrder{}, &db.LocalPlayPaymentTime{}} {
		if err := db.GetIns().AutoMigrate(entity); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []any{&db.AdminUser{ID: 1, Username: "restore-admin", Role: constUtil.AdminRoleAdmin, Enabled: true}, &db.ServerGameClock{ServerID: 1, OffsetDays: -10, Revision: 4}, &db.AdminActivityScheduleRevision{ServerID: 1, Revision: 9}, &db.ClientUpdatePolicy{ID: 1, Maintenance: true}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &GameService{ServerClockReady: true}
	if err := s.InitializeActivityCalendarRestoreSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, &model.AdminPrincipal{UserID: 1, Username: "restore-admin", Role: constUtil.AdminRoleAdmin}
}
func calendarRestoreRequest(t *testing.T, s *GameService) ActivityCalendarRestoreRequest {
	t.Helper()
	real := time.Now()
	plan, err := s.PreviewActivityCalendarRestore(context.Background(), real)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) > 0 {
		t.Fatal(plan.Blockers)
	}
	return ActivityCalendarRestoreRequest{RequestID: "calendar-total-test", ExpectedDigest: plan.Digest, RealMillis: real.UnixMilli()}
}
func TestActivityCalendarRestoreTotalCommitRetryAndDrift(t *testing.T) {
	s, actor := calendarRestoreTransactionFixture(t)
	req := calendarRestoreRequest(t, s)
	// Raw history drift must invalidate the complete reviewed preimage.
	if err := db.GetIns().Model(&db.OpenServerActivityCounter{}).Where("player_id=7").Update("value", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminRestoreActivityCalendar(context.Background(), actor, req); err == nil {
		t.Fatal("history drift accepted")
	}
	req = calendarRestoreRequest(t, s)
	before := openingRestoreSnapshot(t)
	result, err := s.AdminRestoreActivityCalendar(context.Background(), actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Clock.OffsetDays != 0 || result.Clock.Revision != 5 || result.Carries != 2 {
		t.Fatalf("bad result %+v", result)
	}
	replay, err := s.AdminRestoreActivityCalendar(context.Background(), actor, req)
	if err != nil || !reflect.DeepEqual(result, replay) {
		t.Fatal("retry changed result", err)
	}
	after := openingRestoreSnapshot(t)
	for _, index := range []int{3, 4, 5, 6} {
		if !reflect.DeepEqual(before[index], after[index]) {
			t.Fatalf("history %d changed", index)
		}
	}
	if p1Count(t, &db.ServerGameClockOperation{}, "request_id=?", req.RequestID) != 1 {
		t.Fatal("duplicate operation")
	}
}
func TestActivityCalendarRestoreTotalLateFailureRollsBack(t *testing.T) {
	s, actor := calendarRestoreTransactionFixture(t)
	req := calendarRestoreRequest(t, s)
	before := openingRestoreSnapshot(t)
	name := "calendar-restore-late-failure"
	if err := db.GetIns().Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "admin_audits" {
			tx.AddError(errors.New("injected final audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Create().Remove(name)
	if _, err := s.AdminRestoreActivityCalendar(context.Background(), actor, req); err == nil || !strings.Contains(err.Error(), "injected final audit failure") {
		t.Fatal("final injection was not reached", err)
	}
	if !reflect.DeepEqual(before, openingRestoreSnapshot(t)) {
		t.Fatal("late failure leaked calendar writes")
	}
	for _, entity := range []any{&db.OpeningCalendarCarry{}, &db.OpeningCalendarPaymentCarry{}, &db.ServerGameClockOperation{}} {
		var n int64
		if err := db.GetIns().Model(entity).Count(&n).Error; err != nil || n != 0 {
			t.Fatal("late failure leaked carry/operation", err, n)
		}
	}
	var clock db.ServerGameClock
	db.GetIns().First(&clock, "server_id=1")
	if clock.OffsetDays != -10 || clock.Revision != 4 {
		t.Fatal("late failure leaked clock")
	}
}

func TestActivityCalendarRestoreTotalPaymentOwnerWithoutProgress(t *testing.T) {
	s, actor := calendarRestoreTransactionFixture(t)
	for _, entity := range []any{&db.PeriodInstance{}, &db.PeriodProgress{}, &db.PeriodSettlement{}, &db.RewardDelivery{}} {
		if err := db.GetIns().AutoMigrate(entity); err != nil {
			t.Fatal(err)
		}
	}
	start := openingCarryTestDay("2026-10-01")
	cfg := db.PeriodInstance{ID: "paid-retained", Kind: "payment_amount", Mode: "cumulative", Enabled: true, Goal: 100, Rewards: "coin,1", StartAt: start + 4*86400000, EndAt: start + 18*86400000, RewardEndAt: start + 19*86400000, RewardSourceID: "retained-source"}
	cfg.Frozen = adminJSON(cfg)
	source := db.AdminActivityOccurrence{ID: "paid-source", ServerID: 1, ConsumerKey: cfg.ID, ConsumerID: "period:" + cfg.ID, Mode: "recharge-total", Timing: "character-day", StartDay: 5, DurationDays: 14, ClaimDays: 1, StartAt: cfg.StartAt, EndAt: cfg.EndAt, ClaimEndAt: cfg.RewardEndAt, Enabled: true, Revision: 1, FormJSON: adminJSON(map[string]any{"mode": "recharge-total", "startAt": cfg.StartAt, "endAt": cfg.EndAt, "claimEndAt": cfg.RewardEndAt})}
	for _, row := range []any{&cfg, &source, &db.PaymentOrder{ID: "retained-payment", PlayerID: 7, RequestKey: 1, Cents: 300, Status: constUtil.P3PaymentConfirmed, ConfirmedAt: start + 5*86400000, Snapshot: "{}"}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	fake := db.PeriodProgress{InstanceID: cfg.ID, PlayerID: 999, Snapshot: adminJSON(cfg)}
	if err := db.GetIns().Create(&fake).Error; err != nil {
		t.Fatal(err)
	}
	rejected := calendarRestoreRequest(t, s)
	if _, err := s.AdminRestoreActivityCalendar(context.Background(), actor, rejected); err == nil || !strings.Contains(err.Error(), "foreign player") {
		t.Fatal("foreign native ownership accepted", err)
	}
	if err := db.GetIns().Where("instance_id=? AND player_id=?", cfg.ID, 999).Delete(&db.PeriodProgress{}).Error; err != nil {
		t.Fatal(err)
	}
	req := calendarRestoreRequest(t, s)
	result, err := s.AdminRestoreActivityCalendar(context.Background(), actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Carries != 4 {
		t.Fatalf("missing legal no-progress owners: %+v", result)
	}
	var carry db.ActivityRechargeCarry
	if err := db.GetIns().First(&carry, "instance_id=? AND player_id=?", cfg.ID, 7).Error; err != nil {
		t.Fatal(err)
	}
	if carry.OpeningAt != result.OpeningAt+4*86400000 {
		t.Fatal("future consumer lost its own target start")
	}
	if !strings.Contains(string(carry.Facts), "retained-payment") {
		t.Fatal("paid qualification absent without progress")
	}
	if p1Count(t, &db.PeriodProgress{}, "instance_id=?", cfg.ID) != 0 {
		t.Fatal("restore invented progress")
	}
	if p1Count(t, &db.ActivityRechargeCarry{}, "instance_id=? AND player_id=?", cfg.ID, 1) != 0 {
		t.Fatal("protected role acquired carry")
	}
}

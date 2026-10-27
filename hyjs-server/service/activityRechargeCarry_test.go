package service

import (
	"errors"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestActivityRechargeCarryModesDedupAndDayRollover(t *testing.T) {
	opening := time.Date(2026, 10, 27, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)).UnixMilli()
	row := db.ActivityRechargeCarry{PlayerID: 7, RealCutoff: opening + 1000, OpeningAt: opening}
	cfg := db.PeriodInstance{StartAt: opening, EndAt: opening + 3*86400000}
	facts := []ActivityRechargeCarryFact{{"old", 600, opening - 86400000, opening}, {"prior-day", 900, opening - 2*86400000, 0}}
	orders := []db.PaymentOrder{{ID: "old", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 600, ConfirmedAt: opening + 2000}, {ID: "new", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 400, ConfirmedAt: opening + 3000}, {ID: "before-cutoff", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 20000, ConfirmedAt: opening - 1}}
	for _, test := range []struct {
		mode string
		want int64
	}{{"cumulative", 1000}, {"single", 600}, {"daily", 1000}} {
		cfg.Mode = test.mode
		got, day, err := activityRechargeCarryValue(row, cfg, facts, orders, opening+4000)
		if err != nil || got != test.want || day != "2026-10-27" {
			t.Fatalf("%s got=%d day=%s err=%v", test.mode, got, day, err)
		}
	}
	cfg.Mode = "daily"
	got, day, err := activityRechargeCarryValue(row, cfg, facts, orders, opening+86400000)
	if err != nil || got != 0 || day != "2026-10-28" {
		t.Fatalf("day reset: %d %s %v", got, day, err)
	}
	orders = append(orders, db.PaymentOrder{ID: "tomorrow", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 123, ConfirmedAt: opening + 86400001})
	got, day, err = activityRechargeCarryValue(row, cfg, facts, orders, opening+86400002)
	if err != nil || got != 123 || day != "2026-10-28" {
		t.Fatalf("new daily: %d %s %v", got, day, err)
	}
}
func TestActivityRechargeCarryRejectsTamperedFactsAndOverflow(t *testing.T) {
	row := db.ActivityRechargeCarry{PlayerID: 7, RealCutoff: 100, OpeningAt: 100}
	cfg := db.PeriodInstance{Mode: "cumulative", StartAt: 100, EndAt: 1000}
	for _, facts := range [][]ActivityRechargeCarryFact{{{"a", 1, 1, 100}, {"a", 1, 1, 100}}, {{"a", -1, 1, 100}}, {{"a", math.MaxInt64, 1, 100}, {"b", 1, 1, 100}}} {
		if _, _, err := activityRechargeCarryValue(row, cfg, facts, nil, 200); err == nil {
			t.Fatalf("invalid facts: %v", facts)
		}
	}
	row.Facts = []byte("[]")
	row.Digest = activityRechargeCarryDigest(row)
	row.OpeningAt++
	if row.Digest == activityRechargeCarryDigest(row) {
		t.Fatal("digest did not bind restored day")
	}
}

func TestActivityRechargeCarryRequiresTransactionBeforeReadOrWrite(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "carry-guard.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = conn.AutoMigrate(&db.ActivityRechargeCarry{}, &db.PaymentOrder{}); err != nil {
		t.Fatal(err)
	}
	paid := db.PaymentOrder{ID: "accepted", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 600, ConfirmedAt: 1000}
	if err = conn.Create(&paid).Error; err != nil {
		t.Fatal(err)
	}
	var queries int
	if err = conn.Callback().Query().Before("gorm:query").Register("observe_carry_guard_queries", func(*gorm.DB) { queries++ }); err != nil {
		t.Fatal(err)
	}
	before := db.PeriodInstance{ID: "carry-guard", Kind: "payment_amount", Mode: "cumulative", StartAt: 1, EndAt: 2000}
	row := db.ActivityRechargeCarry{InstanceID: before.ID, PlayerID: 7, RequestID: "restore", OpeningAt: 3000, RealCutoff: 2500, Mode: before.Mode, Facts: []byte("[]"), Baseline: []byte("[]")}
	row.Digest = activityRechargeCarryDigest(row)
	for _, test := range []struct {
		name string
		conn *gorm.DB
	}{{"nil", nil}, {"empty", &gorm.DB{}}, {"bare", conn}} {
		t.Run(test.name, func(t *testing.T) {
			carry, err := PrepareActivityRechargeCarry(test.conn, 7, before, "restore", 2000, 2500, 3000)
			if !errors.Is(err, constUtil.ErrAdminConfiguration) || carry != nil {
				t.Fatalf("nontransactional preparation returned carry=%+v err=%v", carry, err)
			}
			if err := SaveActivityRechargeCarry(test.conn, &row); !errors.Is(err, constUtil.ErrAdminConfiguration) {
				t.Fatalf("nontransactional save: %v", err)
			}
		})
	}
	if queries != 0 {
		t.Fatalf("qualification read outside transaction: %d queries", queries)
	}
	var count int64
	if err = conn.Model(&db.ActivityRechargeCarry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("bare calls wrote carry: count=%d err=%v", count, err)
	}
	var saved db.PaymentOrder
	if err = conn.First(&saved, "id=?", paid.ID).Error; err != nil || saved != paid {
		t.Fatalf("accepted payment changed: %+v err=%v", saved, err)
	}
}

// This temporary SQLite contract test never connects to the shared MySQL service.
func TestActivityRechargeCarryTransactionAndSharedEligibility(t *testing.T) {
	tx, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "carry.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := tx.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = tx.AutoMigrate(&db.ActivityRechargeCarry{}, &db.PaymentOrder{}, &db.PeriodInstance{}, &db.PeriodProgress{}); err != nil {
		t.Fatal(err)
	}
	opening := time.Date(2026, 10, 27, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)).UnixMilli()
	old := db.PeriodInstance{ID: "rechargeTotal_p4_restore_carry", Kind: "payment_amount", Mode: "cumulative", StartAt: opening - 3*86400000, EndAt: opening, RewardEndAt: opening + 86400000, Goal: 600, Rewards: "coin,1"}
	paid := db.PaymentOrder{ID: "old-paid", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 600, ConfirmedAt: opening - 1000}
	if err = tx.Create(&paid).Error; err != nil {
		t.Fatal(err)
	}
	var carry *db.ActivityRechargeCarry
	prepareAndSave := func(inner *gorm.DB) error {
		var prepareErr error
		carry, prepareErr = PrepareActivityRechargeCarry(inner, 7, old, "restore-1", opening, opening+1000, opening)
		if prepareErr != nil {
			return prepareErr
		}
		return SaveActivityRechargeCarry(inner, carry)
	}
	sentinel := errors.New("interrupt restore")
	if err = tx.Transaction(func(inner *gorm.DB) error {
		if err := prepareAndSave(inner); err != nil {
			return err
		}
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	var count int64
	if err = tx.Model(&db.ActivityRechargeCarry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rollback count=%d err=%v", count, err)
	}
	if err = tx.Transaction(prepareAndSave); err != nil {
		t.Fatal(err)
	}
	save := func(row *db.ActivityRechargeCarry) error {
		return tx.Transaction(func(inner *gorm.DB) error { return SaveActivityRechargeCarry(inner, row) })
	}
	if err = save(carry); err != nil {
		t.Fatal("same identity retry", err)
	}
	changedInput := *carry
	changedInput.OpeningAt++
	changedInput.Digest = activityRechargeCarryDigest(changedInput)
	if err = save(&changedInput); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("same request changed input: %v", err)
	}
	changed := *carry
	changed.RequestID = "different"
	changed.Digest = activityRechargeCarryDigest(changed)
	if err = save(&changed); err == nil {
		t.Fatal("accepted another restore")
	}
	restored := old
	restored.StartAt = opening
	// The accepted old order is now outside the shortened window and remains carried.
	restored.EndAt = opening + 3*86400000
	restored.RewardEndAt = opening + 4*86400000
	if err = tx.Create(&restored).Error; err != nil {
		t.Fatal(err)
	}
	newPaid := db.PaymentOrder{ID: "new-paid", RequestKey: 2, PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 400, ConfirmedAt: opening + 2000}
	if err = tx.Create(&newPaid).Error; err != nil {
		t.Fatal(err)
	}
	got, _, err := rechargePeriodValue75(tx, 7, restored, opening+3000)
	if err != nil || got != 1000 {
		t.Fatalf("shared progress got=%d err=%v", got, err)
	}
	rows := []db.RewardDelivery{{ID: 11, PlayerID: 7, SourceKey: "activity:" + old.ID}, {ID: 12, PlayerID: 7, SourceKey: "activity:" + old.ID, ClaimedAt: opening - 500}}
	eligible, err := rechargeRewardEligibility75(tx, rows, opening+3000)
	if err != nil || !eligible[11] || !eligible[12] {
		t.Fatalf("shared eligibility=%v err=%v", eligible, err)
	}
	if rows[1].ClaimedAt != opening-500 {
		t.Fatal("claimed receipt changed")
	}
}

func TestActivityRechargeCarryProjectedOrdersAndShortWindow(t *testing.T) {
	tx, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "projected.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := tx.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = tx.AutoMigrate(&db.ActivityRechargeCarry{}, &db.PaymentOrder{}, &db.LocalPlayPaymentTime{}); err != nil {
		t.Fatal(err)
	}
	opening := time.Date(2026, 10, 20, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)).UnixMilli()
	real := opening + 7*86400000
	cfg := db.PeriodInstance{ID: "carry-short", Mode: "cumulative", StartAt: opening, EndAt: opening + 3*86400000}
	row := db.ActivityRechargeCarry{InstanceID: cfg.ID, PlayerID: 7, RequestID: "restore", Mode: cfg.Mode, OpeningAt: opening, RealCutoff: real, Facts: []byte("[]"), Baseline: []byte("[]")}
	row.Digest = activityRechargeCarryDigest(row)
	if err = tx.Transaction(func(inner *gorm.DB) error { return SaveActivityRechargeCarry(inner, &row) }); err != nil {
		t.Fatal(err)
	}
	for i, item := range []struct {
		id                    string
		realAt, gameAt, cents int64
	}{
		{"pre-cutoff", real - 1, opening + 1, 10000},
		{"at-cutoff", real, opening, 200},
		{"now", real + 1000, opening + 1000, 300},
		{"future", real + 1001, opening + 1001, 40000},
		{"end", real + 3*86400000, cfg.EndAt, 50000},
		{"zero-offset", real + 2000, real + 2000, 700},
	} {
		order := db.PaymentOrder{ID: item.id, PlayerID: 7, RequestKey: int64(i + 1), Status: constUtil.P3PaymentConfirmed, ConfirmedAt: item.realAt, Cents: item.cents}
		if err = tx.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		projection := db.LocalPlayPaymentTime{OrderID: item.id, PlayerID: 7, RealAt: item.realAt, GameAt: item.gameAt, OffsetDays: -7}
		if err = tx.Create(&projection).Error; err != nil {
			t.Fatal(err)
		}
	}
	value, _, carried, err := rechargeCarryValue(tx, 7, cfg, opening+1000)
	if err != nil || !carried || value != 500 {
		t.Fatalf("projected cutoff value=%d carry=%v err=%v", value, carried, err)
	}
	value, _, _, err = rechargeCarryValue(tx, 7, cfg, cfg.EndAt)
	if err != nil || value != 40500 {
		t.Fatalf("end exclusive value=%d err=%v", value, err)
	}
	// A later calendar adjustment reads each original frozen projection once.
	later := cfg
	later.StartAt, later.EndAt = real, real+3*86400000
	value, _, _, err = rechargeCarryValue(tx, 7, later, real+3000)
	if err != nil || value != 700 {
		t.Fatalf("later zero-offset projection value=%d err=%v", value, err)
	}
	var saved db.PaymentOrder
	if err = tx.First(&saved, "id=?", "now").Error; err != nil || saved.ConfirmedAt != real+1000 {
		t.Fatalf("real fact changed: %+v %v", saved, err)
	}
}

func TestActivityRechargeCarryExcludesBaselineAtSameMillisecond(t *testing.T) {
	cfg := db.PeriodInstance{ID: "restore", Mode: "cumulative", StartAt: 1000, EndAt: 2000}
	row := db.ActivityRechargeCarry{PlayerID: 7, RealCutoff: 9000, OpeningAt: 1000, Baseline: adminJSON([]ActivityRechargeCarryFact{{OrderID: "old-accepted", Cents: 100, OriginalAt: 9000}, {OrderID: "old-outside", Cents: 500, OriginalAt: 9000}})}
	facts := []ActivityRechargeCarryFact{{OrderID: "old-accepted", Cents: 100, OriginalAt: 100, EffectiveAt: 1000}}
	orders := []db.PaymentOrder{{ID: "old-outside", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 500, ConfirmedAt: 1001}, {ID: "new-same-ms", PlayerID: 7, Status: constUtil.P3PaymentConfirmed, Cents: 200, ConfirmedAt: 1001}}
	got, _, err := activityRechargeCarryValue(row, cfg, facts, orders, 1500)
	if err != nil || got != 300 {
		t.Fatalf("baseline replay or same-ms new payment lost: %d %v", got, err)
	}
}

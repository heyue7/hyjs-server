package service

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// This opt-in integration test accepts only an explicit local test lease file.
func TestActivityCalendarRestoreTurntableMySQL(t *testing.T) {
	if os.Getenv("HYJS_CALENDAR_MYSQL") != "1" {
		t.Skip("isolated calendar MySQL lease required")
	}
	leaseFile := os.Getenv("HYJS_CALENDAR_MYSQL_ENV")
	if leaseFile == "" {
		t.Fatal("HYJS_CALENDAR_MYSQL_ENV must identify a manage.py test lease")
	}
	file, err := os.Open(leaseFile)
	if err != nil {
		t.Fatal("calendar lease credentials unavailable")
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := strings.Cut(line, "="); ok {
			values[k] = strings.Trim(v, "\"'")
		}
	}
	scanErr := scanner.Err()
	file.Close()
	if scanErr != nil {
		t.Fatal("calendar credential read failed")
	}
	if !regexp.MustCompile(`^hyjs_test_[A-Za-z0-9_]+$`).MatchString(values["MYSQL_DATABASE"]) || len(values["MYSQL_DATABASE"]) > 64 || values["MYSQL_HOST"] != "127.0.0.1" || values["MYSQL_PORT"] != "3306" {
		t.Fatal("refusing non-lease database endpoint")
	}
	cfg := driver.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = "tcp", net.JoinHostPort(values["MYSQL_HOST"], values["MYSQL_PORT"]), values["MYSQL_USER"], values["MYSQL_PASSWORD"], values["MYSQL_DATABASE"]
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 3*time.Second, 10*time.Second, 10*time.Second
	cfg.Params = map[string]string{"charset": "utf8mb4", "sql_mode": "'STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION,NO_AUTO_VALUE_ON_ZERO'"}
	connection, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("isolated calendar MySQL connection failed")
	}
	sqlDB, err := connection.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(2)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error("close calendar connections", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	connection = connection.WithContext(ctx)
	var actual string
	if err := connection.Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != values["MYSQL_DATABASE"] {
		t.Fatal("connected database guard failed")
	}
	models := []any{&db.GameplayPool{}, &db.GameplayPoolReward{}, &db.TurntableRules{}, &db.TurntableGlobal{}, &db.TurntablePersonal{}, &db.TurntableState{}, &db.TurntableRecord{}, &db.TurntableDrawReceipt{}}
	if err := connection.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	before, after := FrozenJSONWindow{1000, 2000, 3000}, FrozenJSONWindow{4000, 5000, 6000}
	pool := db.GameplayPool{ID: fmt.Sprintf("calendar-mysql-%d", time.Now().UnixNano()), Name: "historical", Source: "isolated MySQL test", Enabled: true, MinLevel: 1, SingleCost: "diamond,50", TenCost: "diamond,488", CoinPerDraw: 10000, FreeSeconds: 86400, FreePerDay: -1, StartAt: 1000, EndAt: 2000, RewardEndAt: 3000, InitialDiamondPool: 10000, JackpotPerMillion: 200, PoolContributionBps: 200}
	t.Cleanup(func() {
		// Delete only this fixture; retain the leased schema for the parent owner.
		for i := len(models) - 1; i >= 0; i-- {
			key := "pool_id"
			if i == 0 {
				key = "id"
			}
			if err := connection.WithContext(context.Background()).Where(key+"=?", pool.ID).Delete(models[i]).Error; err != nil {
				t.Error("fixture cleanup", err)
			}
		}
	})
	entities := []any{&pool, &db.TurntableRules{PoolID: pool.ID, ScorePerDraw: 1, PrizePityRules: "[]"}, &db.TurntableGlobal{PoolID: pool.ID, DiamondPool: 9999, ContributionRemainder: 123, UsedStock: []byte(`{ "0": 1 }`), FinalRanks: []byte("[]")}, &db.TurntablePersonal{PoolID: pool.ID, PlayerID: 7, DiamondPool: 1234, ContributionRemainder: 456, UsedStock: []byte(`{ "0": 2 }`), LegacyScore: 8, CreatedAt: 999}, &db.TurntableState{PoolID: pool.ID, PlayerID: 7, Score: 12, BoughtCoins: 120000, NextFree: 12345, FreeDay: "2026-10-27", FreeUsed: 1, Timestamp: 100, RequestCount: 10, Response: []byte{8, 10}, PrizePityState: `{"0":{"key":"historical-rule","since":3}}`}, &db.TurntableRecord{PoolID: pool.ID, PlayerID: 7, ResourceID: "coin", Amount: 10000, CreatedAtMillis: 123}, &db.TurntableDrawReceipt{PoolID: pool.ID, PlayerID: 7, Stamp: 100, Count: 10, Response: []byte{8, 10}}}
	for i := int32(0); i < 9; i++ {
		entities = append(entities, &db.GameplayPoolReward{PoolID: pool.ID, ID: i, ResourceID: "coin", Amount: 1, Weight: 10, Stock: 5})
	}
	for _, entity := range entities {
		if err := connection.Create(entity).Error; err != nil {
			t.Fatal(err)
		}
	}

	snapshot := func() []any {
		t.Helper()
		var pools []db.GameplayPool
		var rewards []db.GameplayPoolReward
		var rules []db.TurntableRules
		var globals []db.TurntableGlobal
		var personals []db.TurntablePersonal
		var states []db.TurntableState
		var records []db.TurntableRecord
		var receipts []db.TurntableDrawReceipt
		targets := []any{&pools, &rewards, &rules, &globals, &personals, &states, &records, &receipts}
		for i, target := range targets {
			key := "pool_id"
			if i == 0 {
				key = "id"
			}
			if err := connection.Where(key+"=?", pool.ID).Order("1").Find(target).Error; err != nil {
				t.Fatal(err)
			}
		}
		return []any{pools, rewards, rules, globals, personals, states, records, receipts}
	}
	original := snapshot()
	rollback := errors.New("later migration rejected")
	err = connection.Transaction(func(tx *gorm.DB) error {
		if err := RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) || !reflect.DeepEqual(original, snapshot()) {
		t.Fatal("outer rollback failed to preserve historical state", err)
	}
	// Hold the first successful restore uncommitted. A competing stale restore must
	// wait on InnoDB's row lock and then reject the newly committed window.
	first := connection.Begin()
	if first.Error != nil {
		t.Fatal(first.Error)
	}
	defer first.Rollback()
	if err := RestoreActivityCalendarTurntableWindow(first, pool.ID, before, after); err != nil {
		t.Fatal(err)
	}
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- connection.Transaction(func(tx *gorm.DB) error {
			close(entered)
			return RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, FrozenJSONWindow{7000, 8000, 9000})
		})
	}()
	<-entered
	select {
	case err := <-done:
		t.Fatalf("competing restore escaped held row lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := first.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatal("stale competing restore did not conflict", err)
		}
	case <-ctx.Done():
		t.Fatal("competing restore did not finish after lock release")
	}
	expected := original[0].([]db.GameplayPool)
	expected[0].StartAt, expected[0].EndAt, expected[0].RewardEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
	if !reflect.DeepEqual(original, snapshot()) {
		t.Fatal("commit changed non-window columns, state, stock, records or receipt bytes")
	}
	if err := connection.Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after) }); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale CAS accepted", err)
	}
	if !reflect.DeepEqual(original, snapshot()) {
		t.Fatal("stale restore changed committed state")
	}
	t.Log("MySQL outer rollback, window-only commit, history/receipt bytes, held row lock and stale CAS verified")
	activityCalendarScheduleRestoreMySQL(t, connection)
	activityCalendarOpeningCarryMySQL(t, connection)
}

func activityCalendarScheduleMySQLFixture(t *testing.T, connection *gorm.DB, mode string) (*ActivityCalendarRestorePlan, []db.AdminActivityOccurrence, []db.AdminCharacterActivityInstance) {
	t.Helper()
	day := int64(86400000)
	start := openingCarryTestDay("2026-10-01")
	duration := int64(3)
	nextDuration := int64(3)
	if mode == "opening-14" {
		nextDuration = 14
	}
	form := func(start, duration int64, extra string) []byte {
		return []byte(`{"mode":"` + mode + `","startAt":` + string(adminJSON(start)) + `,"endAt":` + string(adminJSON(start+duration*day)) + `,"claimEndAt":` + string(adminJSON(start+(duration+1)*day)) + `,"unknown":{"large":9007199254740993,"label":"` + extra + `"},"rewards":[{"id":"coin","amount":7}]}`)
	}
	source := db.AdminActivityOccurrence{ID: fmt.Sprintf("occ-mysql-%d", time.Now().UnixNano()), ServerID: 1, Timing: "character-day", StartDay: 1, DurationDays: duration, ClaimDays: 1, TemplateID: "template", TemplateVersion: "original-template", Name: "source", Mode: mode, ConsumerID: "character-plan:source", ConsumerKey: "source", StartAt: start, EndAt: start + duration*day, ClaimEndAt: start + (duration+1)*day, Enabled: true, Revision: 9, FirstPublishedAt: 123, FormJSON: form(start+day, duration, "source-private")}
	instance := db.AdminCharacterActivityInstance{PlayerID: 7, OccurrenceID: source.ID, ServerID: 1, ConsumerID: "period:own", ConsumerKey: "own", Mode: mode, StartAt: start + 4*day, EndAt: start + (4+duration)*day, ClaimEndAt: start + (5+duration)*day, SourceVersion: "accepted-older-version", FormJSON: form(start+4*day, duration, "frozen-private")}
	if mode == "opening-14" {
		instance.ConsumerID = "opening:opening_character_1_7"
		instance.ConsumerKey = "opening_character_1_7"
		instance.FormJSON = form(start-30*day, duration, "old-source-frozen")
	}

	source.ConsumerID, source.ConsumerKey = "character-plan:"+source.ID, source.ID
	instance.ConsumerID, instance.ConsumerKey = "period:"+source.ID, "own-"+source.ID
	if mode == "opening-14" {
		instance.ConsumerID = "opening:" + source.ID
	}
	t.Cleanup(func() {
		for _, model := range []any{&db.AdminCharacterActivityInstance{}, &db.AdminActivityOccurrence{}} {
			key := "occurrence_id"
			if _, ok := model.(*db.AdminActivityOccurrence); ok {
				key = "id"
			}
			if err := connection.WithContext(context.Background()).Where(key+"=?", source.ID).Delete(model).Error; err != nil {
				t.Error("schedule fixture cleanup", err)
			}
		}
	})
	for _, row := range []any{&source, &instance} {
		if err := connection.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Exact persisted images include defaults and every hidden identity field.
	if err := connection.Where("id=?", source.ID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := connection.Where("player_id=? AND occurrence_id=?", instance.PlayerID, instance.OccurrenceID).Take(&instance).Error; err != nil {
		t.Fatal(err)
	}
	next := openingCarryTestDay("2026-10-27")
	after := [3]int64{next, next + nextDuration*day, next + (nextDuration+1)*day}
	plan := &ActivityCalendarRestorePlan{ServerID: 1, Windows: []ActivityCalendarRestoreWindow{{OccurrenceID: source.ID, ConsumerID: source.ConsumerID, Mode: source.Mode, SourceVersion: adminVersion(source), Before: [3]int64{source.StartAt, source.EndAt, source.ClaimEndAt}, After: after, StartDay: 1, DurationDays: nextDuration, ClaimDays: 1}, {OccurrenceID: source.ID, PlayerID: 7, ConsumerID: instance.ConsumerID, Mode: instance.Mode, SourceVersion: instance.SourceVersion, Before: [3]int64{instance.StartAt, instance.EndAt, instance.ClaimEndAt}, After: after, StartDay: 1, DurationDays: nextDuration, ClaimDays: 1}}}
	return plan, []db.AdminActivityOccurrence{source}, []db.AdminCharacterActivityInstance{instance}
}

func activityCalendarScheduleRestoreMySQL(t *testing.T, connection *gorm.DB) {
	t.Helper()
	if err := connection.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"admin_activity_occurrences", "admin_character_activity_instances"} {
		var engine string
		if err := connection.Raw("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", table).Scan(&engine).Error; err != nil || engine != "InnoDB" {
			t.Fatal("schedule fixture requires InnoDB", err)
		}
	}
	for _, mode := range []string{"recharge-total", "opening-14"} {
		for _, scenario := range []string{"commit", "outer-rollback", "source-preimage", "issued-preimage", "issued-ordinary-json-drift"} {
			if mode == "opening-14" && scenario == "issued-ordinary-json-drift" {
				continue
			}
			t.Run("schedule/"+mode+"/"+scenario, func(t *testing.T) {
				plan, sources, issued := activityCalendarScheduleMySQLFixture(t, connection, mode)
				read := func() (db.AdminActivityOccurrence, db.AdminCharacterActivityInstance) {
					t.Helper()
					var source db.AdminActivityOccurrence
					var instance db.AdminCharacterActivityInstance
					if err := connection.Where("id=?", sources[0].ID).Take(&source).Error; err != nil {
						t.Fatal(err)
					}
					if err := connection.Where("occurrence_id=? AND player_id=?", issued[0].OccurrenceID, issued[0].PlayerID).Take(&instance).Error; err != nil {
						t.Fatal(err)
					}
					return source, instance
				}
				switch scenario {
				case "source-preimage":
					if err := connection.Model(&db.AdminActivityOccurrence{}).Where("id=?", sources[0].ID).Update("form_json", append([]byte(" "), sources[0].FormJSON...)).Error; err != nil {
						t.Fatal(err)
					}
				case "issued-preimage":
					if err := connection.Model(&db.AdminCharacterActivityInstance{}).Where("occurrence_id=?", sources[0].ID).Update("source_version", "concurrently-changed").Error; err != nil {
						t.Fatal(err)
					}
				case "issued-ordinary-json-drift":
					issued[0].FormJSON = append([]byte(nil), sources[0].FormJSON...)
					if err := connection.Model(&db.AdminCharacterActivityInstance{}).Where("occurrence_id=?", sources[0].ID).Update("form_json", issued[0].FormJSON).Error; err != nil {
						t.Fatal(err)
					}
				}
				beforeSource, beforeIssued := read()
				rollback := errors.New("subsequent consumer migration rejected")
				err := connection.Transaction(func(tx *gorm.DB) error {
					if err := RestoreActivityCalendarScheduleWindows(tx, plan, sources, issued); err != nil {
						return err
					}
					if scenario == "outer-rollback" {
						return rollback
					}
					return nil
				})
				afterSource, afterIssued := read()
				if scenario != "commit" {
					if scenario == "outer-rollback" {
						if !errors.Is(err, rollback) {
							t.Fatal("expected outer rollback", err)
						}
					} else if !errors.Is(err, constUtil.ErrAdminConfigConflict) {
						t.Fatal("expected preimage conflict", err)
					}
					if !reflect.DeepEqual(beforeSource, afterSource) || !reflect.DeepEqual(beforeIssued, afterIssued) {
						t.Fatal("failed restore changed source or issued rows")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				for _, pair := range [][2][]byte{{beforeSource.FormJSON, afterSource.FormJSON}, {beforeIssued.FormJSON, afterIssued.FormJSON}} {
					if !reflect.DeepEqual(activityScheduleRestoreNonDates(t, pair[0]), activityScheduleRestoreNonDates(t, pair[1])) {
						t.Fatal("frozen non-time JSON changed")
					}
				}
				expectedSource, expectedIssued := beforeSource, beforeIssued
				window := plan.Windows[0]
				expectedSource.StartAt, expectedSource.EndAt, expectedSource.ClaimEndAt = window.After[0], window.After[1], window.After[2]
				expectedSource.StartDay, expectedSource.DurationDays, expectedSource.ClaimDays = window.StartDay, window.DurationDays, window.ClaimDays
				expectedSource.Revision++
				expectedSource.FormJSON = afterSource.FormJSON
				expectedIssued.StartAt, expectedIssued.EndAt, expectedIssued.ClaimEndAt = window.After[0], window.After[1], window.After[2]
				expectedIssued.FormJSON = afterIssued.FormJSON
				if !reflect.DeepEqual(expectedSource, afterSource) || !reflect.DeepEqual(expectedIssued, afterIssued) || afterIssued.SourceVersion != "accepted-older-version" {
					t.Fatal("non-calendar fields or historical provenance changed")
				}
				for _, raw := range [][]byte{afterSource.FormJSON, afterIssued.FormJSON} {
					w, err := activityScheduleJSONWindow(raw, mode)
					if err != nil || w != (FrozenJSONWindow{window.After[0], window.After[1], window.After[2]}) {
						t.Fatal("source/issued JSON did not reach target window", err)
					}
				}
				err = connection.Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarScheduleWindows(tx, plan, sources, issued) })
				if !errors.Is(err, constUtil.ErrAdminConfigConflict) {
					t.Fatal("stale schedule preimage accepted", err)
				}
				finalSource, finalIssued := read()
				if !reflect.DeepEqual(afterSource, finalSource) || !reflect.DeepEqual(afterIssued, finalIssued) {
					t.Fatal("stale retry changed committed schedule")
				}
			})
		}
	}
}

func activityCalendarOpeningCarryMySQL(t *testing.T, connection *gorm.DB) {
	t.Helper()
	models := []any{&db.OpeningCalendarPaymentCarry{}, &db.OpeningCalendarCarry{}, &db.PaymentOrder{}, &db.LocalPlayPaymentTime{}, &db.AdminOpeningCounter{}, &db.OpenServerActivityCounter{}}
	if err := connection.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	player := time.Now().UnixNano()
	prefix := fmt.Sprintf("carry-%d", player)
	// Unique player and occurrence identities bound every write/cleanup to this fixture.
	defer func() {
		for _, model := range models {
			if err := connection.WithContext(context.Background()).Where("player_id=?", player).Delete(model).Error; err != nil {
				t.Error("carry fixture cleanup", err)
			}
		}
	}()
	day := int64(86400000)
	start := openingCarryTestDay("2026-10-01")
	before := db.OpenServerActivityRule{ID: prefix, StartAt: start, EndAt: start + 14*day, RewardEndAt: start + 15*day}
	after := before
	after.StartAt += 26 * day
	after.EndAt += 26 * day
	after.RewardEndAt += 26 * day
	cutoff := after.StartAt + 1000
	bind := func(tx *gorm.DB, scope string) {
		adminBindOpeningPeriod(tx, db.AdminCharacterActivityInstance{PlayerID: player, OccurrenceID: scope, ConsumerKey: "opening_period_mysql"})
	}
	for _, pair := range []struct {
		scope string
		value int32
	}{{prefix + "-a", 7}, {prefix + "-b", 11}} {
		if err := connection.Create(&db.AdminOpeningCounter{OccurrenceID: pair.scope, PlayerID: player, CounterKey: "copy_win:2026-10-01", Value: pair.value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := connection.Create(&db.OpenServerActivityCounter{PlayerID: player, CounterKey: "copy_win:2026-10-01", Value: 2}).Error; err != nil {
		t.Fatal(err)
	}
	// Establish RR snapshot before another transaction commits confirmed payment.
	rr := connection.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if rr.Error != nil {
		t.Fatal(rr.Error)
	}
	defer rr.Rollback()
	var n int64
	if err := rr.Model(&db.PaymentOrder{}).Where("player_id=?", player).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("RR pre-snapshot", err)
	}
	if err := connection.Transaction(func(writer *gorm.DB) error {
		for i, money := range []int64{100, 200, 90000} {
			gameAt := before.StartAt + int64(i+1)*1000
			if i == 2 {
				gameAt = after.StartAt + 500
			}
			order := db.PaymentOrder{ID: fmt.Sprintf("%s-old-%d", prefix, i), PlayerID: player, RequestKey: int64(i + 1), Cents: money, ConfirmedAt: cutoff, Status: constUtil.P3PaymentConfirmed, Snapshot: "{}"}
			if err := writer.Create(&order).Error; err != nil {
				return err
			}
			if err := writer.Create(&db.LocalPlayPaymentTime{OrderID: order.ID, PlayerID: player, RealAt: cutoff, GameAt: gameAt}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := rr.Model(&db.PaymentOrder{}).Where("player_id=?", player).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("RR consistent read unexpectedly saw newly committed orders", err)
	}
	bind(rr, prefix+"-a")
	payment, err := PrepareOpeningCalendarPaymentCarry(rr, player, before, after, "restore", before.EndAt, cutoff)
	if err != nil {
		t.Fatal("FOR SHARE prepare failed on RR old snapshot", err)
	}
	var facts, baseline []OpeningCalendarPaymentFact
	if json.Unmarshal(payment.Baseline, &baseline) != nil || json.Unmarshal(payment.Facts, &facts) != nil || len(baseline) != 3 || len(facts) != 2 {
		t.Fatal("FOR SHARE missed committed baseline/payment facts")
	}
	counters, err := PrepareOpeningCalendarCarry(rr, player, "restore", FrozenJSONWindow{before.StartAt, before.EndAt, before.RewardEndAt}, FrozenJSONWindow{after.StartAt, after.EndAt, after.RewardEndAt}, before.EndAt, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveOpeningCalendarPaymentCarry(rr, payment); err != nil {
		t.Fatal(err)
	}
	if err := SaveOpeningCalendarCarry(rr, counters); err != nil {
		t.Fatal(err)
	}
	if err := rr.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&db.OpeningCalendarPaymentCarry{}, &db.OpeningCalendarCarry{}} {
		if err := connection.Model(model).Where("player_id=?", player).Count(&n).Error; err != nil || n != 0 {
			t.Fatal("outer rollback retained carry", err)
		}
	}
	// Save both already prepared images, then prove exact retries do not duplicate them.
	for i := 0; i < 2; i++ {
		if err := connection.Transaction(func(tx *gorm.DB) error {
			bind(tx, prefix+"-a")
			if err := SaveOpeningCalendarPaymentCarry(tx, payment); err != nil {
				return err
			}
			return SaveOpeningCalendarCarry(tx, counters)
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A new order confirmed in the exact same millisecond must qualify once by ID;
	// previously confirmed out-of-window money remains excluded by its baseline ID.
	order := db.PaymentOrder{ID: prefix + "-new", PlayerID: player, RequestKey: 4, Cents: 250, ConfirmedAt: cutoff, Status: constUtil.P3PaymentConfirmed, Snapshot: "{}"}
	if err := connection.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		return tx.Create(&db.LocalPlayPaymentTime{OrderID: order.ID, PlayerID: player, RealAt: cutoff, GameAt: after.StartAt + 3000}).Error
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := connection.Transaction(func(tx *gorm.DB) error {
			bind(tx, prefix+"-a")
			paid, carried, err := openingCalendarPaymentCounters(tx, player, &after, time.UnixMilli(after.StartAt+5000))
			if err != nil {
				return err
			}
			if !carried || paid[constUtil.P3OpenServerTotalPaid] != 5 || paid[constUtil.P3OpenServerSinglePaid+p3Day(time.UnixMilli(after.StartAt))] != 2 {
				return errors.New("same-millisecond order identity qualification mismatch")
			}
			raw, err := openingCarryRawCounters(tx, player, "occurrence:"+prefix+"-a")
			if err != nil {
				return err
			}
			rows, err := openingCalendarCarryCounters(tx, player, &after, raw)
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].CounterKey != "copy_win:2026-10-27" || rows[0].Value != 7 {
				return errors.New("stored counter carry read mismatch")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := connection.Transaction(func(tx *gorm.DB) error {
		bind(tx, prefix+"-b")
		_, carried, err := openingCalendarPaymentCounters(tx, player, &after, time.UnixMilli(after.StartAt+5000))
		if err != nil {
			return err
		}
		if carried {
			return errors.New("payment carry leaked across occurrence")
		}
		raw, err := openingCarryRawCounters(tx, player, "occurrence:"+prefix+"-b")
		if err != nil {
			return err
		}
		rows, err := openingCalendarCarryCounters(tx, player, &after, raw)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].CounterKey != "copy_win:2026-10-01" || rows[0].Value != 11 {
			return errors.New("counter carry leaked across occurrence")
		}
		if err := SaveOpeningCalendarPaymentCarry(tx, payment); err == nil {
			return errors.New("cross-scope payment save accepted")
		}
		if err := SaveOpeningCalendarCarry(tx, counters); err == nil {
			return errors.New("cross-scope counter save accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&db.OpeningCalendarPaymentCarry{}, &db.OpeningCalendarCarry{}} {
		if err := connection.Model(model).Where("player_id=?", player).Count(&n).Error; err != nil || n != 1 {
			t.Fatal("exact carry retry duplicated row", err)
		}
	}
	t.Log("InnoDB carry creation/read, rollback, occurrence isolation, same-ms ID dedupe and FOR SHARE over RR old snapshot verified")
}

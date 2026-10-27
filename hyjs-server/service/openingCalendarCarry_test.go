package service

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func openingCarryTestDay(day string) int64 {
	at, _ := time.ParseInLocation("2006-01-02", day, time.FixedZone("UTC+8", 8*3600))
	return at.UnixMilli()
}
func openingCarryTestFixture(t *testing.T) (*db.OpenServerActivityRule, FrozenJSONWindow, FrozenJSONWindow) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "opening-carry.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	if err := db.GetIns().AutoMigrate(&db.OpeningCalendarCarry{}, &db.OpenServerActivityRule{}, &db.OpenServerActivityCounter{}, &db.AdminOpeningCounter{}); err != nil {
		t.Fatal(err)
	}
	before := FrozenJSONWindow{openingCarryTestDay("2026-10-01"), openingCarryTestDay("2026-10-15"), openingCarryTestDay("2026-10-16")}
	after := FrozenJSONWindow{openingCarryTestDay("2026-10-03"), openingCarryTestDay("2026-10-17"), openingCarryTestDay("2026-10-18")}
	cfg := &db.OpenServerActivityRule{ID: constUtil.OpenServerActivityID, Enabled: true, StartAt: after.StartAt, EndAt: after.EndAt, RewardEndAt: after.ClaimEndAt}
	if err := db.GetIns().Create(cfg).Error; err != nil {
		t.Fatal(err)
	}
	return cfg, before, after
}
func openingCarryTestCounter(t *testing.T, scope, key string, value int32) {
	t.Helper()
	var target any = &db.OpenServerActivityCounter{PlayerID: 7, CounterKey: key, Value: value}
	if scope != "" {
		target = &db.AdminOpeningCounter{OccurrenceID: scope, PlayerID: 7, CounterKey: key, Value: value}
	}
	if err := db.GetIns().Create(target).Error; err != nil {
		t.Fatal(err)
	}
}
func openingCarryTestBind(tx *gorm.DB, scope string) {
	key := "opening_character_1_7"
	if scope != "" {
		key = "opening_period_test"
	}
	adminBindOpeningPeriod(tx, db.AdminCharacterActivityInstance{PlayerID: 7, OccurrenceID: scope, ConsumerKey: key})
}
func openingCarryTestPrepare(t *testing.T, scope string, before, after FrozenJSONWindow) *db.OpeningCalendarCarry {
	t.Helper()
	var row *db.OpeningCalendarCarry
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		openingCarryTestBind(tx, scope)
		var err error
		row, err = PrepareOpeningCalendarCarry(tx, 7, "restore", before, after, openingCarryTestDay("2026-10-05")+1000, openingCarryTestDay("2026-10-27"))
		if err != nil {
			return err
		}
		return SaveOpeningCalendarCarry(tx, row)
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}
func openingCarryTestValues(t *testing.T, scope string, cfg *db.OpenServerActivityRule) map[string]int32 {
	t.Helper()
	values := map[string]int32{}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		openingCarryTestBind(tx, scope)
		rows, err := openServerReadCounters(tx, 7)
		if err != nil {
			return err
		}
		rows, err = openingCalendarCarryCounters(tx, 7, cfg, rows)
		if err != nil {
			return err
		}
		for _, row := range rows {
			values[row.CounterKey] = row.Value
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return values
}

func TestOpeningCalendarCarryOverlapFutureAndAggregation(t *testing.T) {
	cfg, before, after := openingCarryTestFixture(t)
	for key, value := range map[string]int32{"copy_win:2026-10-01": 2, "copy_win:2026-10-03": 4, "copy_win:2026-10-12": 99, "copy_win:2026-10-16": 88, "treasure_predatory:2026-10-02": 3, "login:2026-10-01": 1, "login:2026-10-03": 1, "compose_treasure": 8, "p3_paid_total": 19} {
		openingCarryTestCounter(t, "", key, value)
	}
	var original []db.OpenServerActivityCounter
	if err := db.GetIns().Order("counter_key").Find(&original).Error; err != nil {
		t.Fatal(err)
	}
	carry := openingCarryTestPrepare(t, "", before, after)
	values := openingCarryTestValues(t, "", cfg)
	if values["copy_win:2026-10-03"] != 2 || values["copy_win:2026-10-05"] != 4 || values["copy_win:2026-10-14"] != 0 || values["copy_win:2026-10-16"] != 0 || values["treasure_predatory:2026-10-04"] != 3 || values["compose_treasure"] != 8 || values["p3_paid_total"] != 19 {
		t.Fatal(values)
	}
	// A new event on a previously visited future key contributes only its delta.
	if err := db.GetIns().Model(&db.OpenServerActivityCounter{}).Where("player_id=7 AND counter_key=?", "copy_win:2026-10-16").Update("value", 90).Error; err != nil {
		t.Fatal(err)
	}
	values = openingCarryTestValues(t, "", cfg)
	if values["copy_win:2026-10-16"] != 2 {
		t.Fatal(values)
	}
	s := &GameService{}
	current, err := s.openServerCurrent(nil, nil, cfg, db.OpenServerActivityTask{TaskType: 102, OpenDay: 8}, values)
	if err != nil || current != 8 {
		t.Fatal("period copy sum", current, err)
	}
	current, err = s.openServerCurrent(nil, nil, cfg, db.OpenServerActivityTask{TaskType: 102, OpenDay: 1}, values)
	if err != nil || current != 2 {
		t.Fatal("first-day exact count", current, err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { openingCarryTestBind(tx, ""); return SaveOpeningCalendarCarry(tx, carry) }); err != nil {
		t.Fatal("retry", err)
	}
	var saved []db.OpenServerActivityCounter
	if err := db.GetIns().Order("counter_key").Find(&saved).Error; err != nil {
		t.Fatal(err)
	}
	for i := range original {
		if original[i].CounterKey == "copy_win:2026-10-16" {
			original[i].Value = 90
		}
	}
	if !reflect.DeepEqual(original, saved) {
		t.Fatal("projection modified raw counters")
	}
}

func TestOpeningCalendarCarryLoginOldOneAndRollback(t *testing.T) {
	cfg, before, after := openingCarryTestFixture(t)
	// Old target date is already 1; raw login recording will return early.
	openingCarryTestCounter(t, "", "login:2026-10-03", 1)
	carry := openingCarryTestPrepare(t, "", before, after)
	values := openingCarryTestValues(t, "", cfg)
	if values["login:2026-10-03"] != 0 || values["login:2026-10-05"] != 1 {
		t.Fatal(values)
	}
	now := time.UnixMilli(after.StartAt + 1000)
	rollback := errors.New("later failure")
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		openingCarryTestBind(tx, "")
		if err := recordOpenServerCounterAt(tx, 7, "login:2026-10-03", 1, now); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	values = openingCarryTestValues(t, "", cfg)
	if values["login:2026-10-03"] != 0 {
		t.Fatal("rollback retained login", values)
	}
	for i := 0; i < 2; i++ {
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			openingCarryTestBind(tx, "")
			return recordOpenServerCounterAt(tx, 7, "login:2026-10-03", 1, now)
		}); err != nil {
			t.Fatal(err)
		}
	}
	values = openingCarryTestValues(t, "", cfg)
	if values["login:2026-10-03"] != 1 || values["login:2026-10-05"] != 1 {
		t.Fatal(values)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return SaveOpeningCalendarCarry(tx, carry) }); err != nil {
		t.Fatal("retry after login", err)
	}
	var raw db.OpenServerActivityCounter
	if err := db.GetIns().Where("player_id=7 AND counter_key=?", "login:2026-10-03").Take(&raw).Error; err != nil || raw.Value != 1 {
		t.Fatal("raw login changed", raw, err)
	}
}

func TestOpeningCalendarCarryScopeIsolationAndAbsent(t *testing.T) {
	cfg, before, after := openingCarryTestFixture(t)
	openingCarryTestCounter(t, "", "copy_win:2026-10-01", 2)
	openingCarryTestCounter(t, "period-a", "copy_win:2026-10-01", 7)
	openingCarryTestCounter(t, "period-b", "copy_win:2026-10-01", 11)
	openingCarryTestPrepare(t, "period-a", before, after)
	if values := openingCarryTestValues(t, "period-a", cfg); values["copy_win:2026-10-03"] != 7 {
		t.Fatal(values)
	}
	for scope, want := range map[string]int32{"": 2, "period-b": 11} {
		values := openingCarryTestValues(t, scope, cfg)
		if values["copy_win:2026-10-01"] != want || values["copy_win:2026-10-03"] != 0 {
			t.Fatal(scope, values)
		}
	}
}

func TestOpeningCalendarCarryPrivateCycleScope(t *testing.T) {
	cfg, before, after := openingCarryTestFixture(t)
	if err := db.GetIns().AutoMigrate(&db.LocalPlayActivityBinding{}, &db.LocalPlayActivityCycle{}, &db.LocalPlayOpenServerCounter{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cycle-a", "cycle-b"} {
		cycle := db.LocalPlayActivityCycle{ID: id, PlayerID: 7, ActivityID: "OpenHappy", StartAt: before.StartAt, EndAt: before.EndAt, RewardEndAt: before.ClaimEndAt, Source: constUtil.LocalPlayActivitiesVersion, Snapshot: []byte("{}")}
		if err := db.GetIns().Create(&cycle).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&db.LocalPlayOpenServerCounter{CycleID: id, PlayerID: 7, CounterKey: "copy_win:2026-10-01", Value: 5}).Error; err != nil {
			t.Fatal(err)
		}
	}
	binding := db.LocalPlayActivityBinding{PlayerID: 7, ActivityID: "OpenHappy", CycleID: "cycle-a", Revision: 1}
	if err := db.GetIns().Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), constUtil.LocalPlayBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalPlayOwnerKey, int64(7))
	var carry *db.OpeningCalendarCarry
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		carry, err = PrepareOpeningCalendarCarry(tx, 7, "restore", before, after, before.EndAt, after.StartAt)
		if err != nil {
			return err
		}
		return SaveOpeningCalendarCarry(tx, carry)
	}); err != nil {
		t.Fatal(err)
	}
	if carry.Scope != "cycle:cycle-a" {
		t.Fatal("cycle routed to wrong ledger", carry.Scope)
	}
	check := func(wantKey string) {
		t.Helper()
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			rows, err := openServerReadCounters(tx, 7)
			if err != nil {
				return err
			}
			rows, err = openingCalendarCarryCounters(tx, 7, cfg, rows)
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].CounterKey != wantKey || rows[0].Value != 5 {
				t.Fatal(rows)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check("copy_win:2026-10-03")
	if err := db.GetIns().Model(&binding).Update("cycle_id", "cycle-b").Error; err != nil {
		t.Fatal(err)
	}
	check("copy_win:2026-10-01")
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error { return SaveOpeningCalendarCarry(tx, carry) }); err == nil {
		t.Fatal("carry accepted into another cycle")
	}
}

func TestOpeningCalendarCarryRejectsAndCaptureRollback(t *testing.T) {
	for _, scenario := range []string{"capture-rollback", "bare-db", "negative", "bad-date", "login-two", "different-request", "decreased", "overflow", "scope-conflict", "window-conflict", "digest"} {
		t.Run(scenario, func(t *testing.T) {
			cfg, before, after := openingCarryTestFixture(t)
			initial := int32(2)
			key := "copy_win:2026-10-01"
			if scenario == "negative" {
				initial = -1
			}
			if scenario == "bad-date" {
				key = "copy_win:bad"
			}
			if scenario == "login-two" {
				key = "login:2026-10-01"
			}
			if scenario == "overflow" {
				initial = math.MaxInt32
			}
			openingCarryTestCounter(t, "", key, initial)
			if scenario == "negative" || scenario == "bad-date" || scenario == "login-two" || scenario == "bare-db" || scenario == "capture-rollback" {
				capture := func(tx *gorm.DB) error {
					row, err := PrepareOpeningCalendarCarry(tx, 7, "restore", before, after, openingCarryTestDay("2026-10-05"), openingCarryTestDay("2026-10-27"))
					if err != nil {
						return err
					}
					if err = SaveOpeningCalendarCarry(tx, row); err != nil {
						return err
					}
					return errors.New("caller failure")
				}
				var err error
				if scenario == "bare-db" {
					err = capture(db.GetIns())
				} else {
					err = db.GetIns().Transaction(capture)
				}
				if err == nil {
					t.Fatal("expected refusal")
				}
				var n int64
				db.GetIns().Model(&db.OpeningCalendarCarry{}).Count(&n)
				if n != 0 {
					t.Fatal("partial carry persisted")
				}
				return
			}
			carry := openingCarryTestPrepare(t, "", before, after)
			var err error
			switch scenario {
			case "different-request":
				carry.RequestID = "other"
				carry.Digest = openingCarryDigest(*carry)
				err = db.GetIns().Transaction(func(tx *gorm.DB) error { return SaveOpeningCalendarCarry(tx, carry) })
			case "scope-conflict":
				err = db.GetIns().Transaction(func(tx *gorm.DB) error {
					openingCarryTestBind(tx, "period-a")
					return SaveOpeningCalendarCarry(tx, carry)
				})
			default:
				if scenario == "decreased" {
					if e := db.GetIns().Model(&db.OpenServerActivityCounter{}).Where("player_id=7").Update("value", 1).Error; e != nil {
						t.Fatal(e)
					}
				}
				if scenario == "overflow" {
					openingCarryTestCounter(t, "", "copy_win:2026-10-03", 1)
				}
				if scenario == "window-conflict" {
					cfg.EndAt++
				}
				if scenario == "digest" {
					if e := db.GetIns().Model(&db.OpeningCalendarCarry{}).Where("player_id=7").Update("digest", "bad").Error; e != nil {
						t.Fatal(e)
					}
				}
				err = db.GetIns().Transaction(func(tx *gorm.DB) error {
					rows, e := openServerReadCounters(tx, 7)
					if e != nil {
						return e
					}
					_, e = openingCalendarCarryCounters(tx, 7, cfg, rows)
					return e
				})
			}
			if err == nil {
				t.Fatal("expected refusal")
			}
		})
	}
}

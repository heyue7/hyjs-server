package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/db"
)

func turntableCalendarRestoreFixture(t *testing.T) (db.GameplayPool, FrozenJSONWindow, FrozenJSONWindow) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "turntable-calendar.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	if err := db.GetIns().AutoMigrate(&db.GameplayPool{}, &db.GameplayPoolReward{}, &db.TurntableRules{}, &db.TurntableGlobal{}, &db.TurntablePersonal{}, &db.TurntableState{}, &db.TurntableRecord{}, &db.TurntableDrawReceipt{}); err != nil {
		t.Fatal(err)
	}
	before, after := FrozenJSONWindow{1000, 2000, 3000}, FrozenJSONWindow{4000, 5000, 6000}
	pool := db.GameplayPool{ID: "calendar-wheel", Name: "preserved", Source: "reviewed fixture", Enabled: true, MinLevel: 1, SingleCost: "diamond,50", TenCost: "diamond,488", CoinPerDraw: 10000, FreeSeconds: 86400, FreePerDay: -1, StartAt: before.StartAt, EndAt: before.EndAt, RewardEndAt: before.ClaimEndAt, InitialDiamondPool: 10000, JackpotPerMillion: 200, PoolContributionBps: 200}
	entities := []any{&pool, &db.TurntableRules{PoolID: pool.ID, ScorePerDraw: 1, PrizePityRules: "[]"}, &db.TurntableGlobal{PoolID: pool.ID, DiamondPool: 9999, ContributionRemainder: 123, UsedStock: []byte(`{ "0": 1 }`), FinalRanks: []byte("[]")}, &db.TurntablePersonal{PoolID: pool.ID, PlayerID: 7, DiamondPool: 1234, ContributionRemainder: 456, UsedStock: []byte(`{ "0": 2 }`), LegacyScore: 8, CreatedAt: 999}, &db.TurntableState{PoolID: pool.ID, PlayerID: 7, Score: 12, BoughtCoins: 120000, NextFree: 12345, FreeDay: "2026-10-27", FreeUsed: 1, Timestamp: 100, RequestCount: 10, Response: []byte{8, 10}, PrizePityState: `{"0":{"key":"historical-rule","since":3}}`}, &db.TurntableRecord{PoolID: pool.ID, PlayerID: 7, ResourceID: "coin", Amount: 10000, CreatedAtMillis: 123}, &db.TurntableDrawReceipt{PoolID: pool.ID, PlayerID: 7, Stamp: 100, Count: 10, Response: []byte{8, 10}}}
	for i := int32(0); i < 9; i++ {
		entities = append(entities, &db.GameplayPoolReward{PoolID: pool.ID, ID: i, ResourceID: "coin", Amount: 1, Weight: 10, Stock: 5})
	}
	for _, entity := range entities {
		if err := db.GetIns().Create(entity).Error; err != nil {
			t.Fatal(err)
		}
	}
	return pool, before, after
}

func turntableCalendarRestoreSnapshot(t *testing.T) []any {
	t.Helper()
	var (
		pools     []db.GameplayPool
		rewards   []db.GameplayPoolReward
		rules     []db.TurntableRules
		globals   []db.TurntableGlobal
		personals []db.TurntablePersonal
		states    []db.TurntableState
		records   []db.TurntableRecord
		receipts  []db.TurntableDrawReceipt
	)
	for _, target := range []any{&pools, &rewards, &rules, &globals, &personals, &states, &records, &receipts} {
		if err := db.GetIns().Find(target).Error; err != nil {
			t.Fatal(err)
		}
	}
	return []any{pools, rewards, rules, globals, personals, states, records, receipts}
}

func TestTurntableCalendarRestoreOnlyWindowAndRollback(t *testing.T) {
	for _, quota := range []int32{-1, 0} {
		t.Run(map[int32]string{-1: "cooldown", 0: "paid-only"}[quota], func(t *testing.T) {
			pool, before, after := turntableCalendarRestoreFixture(t)
			if err := db.GetIns().Model(&pool).Update("free_per_day", quota).Error; err != nil {
				t.Fatal(err)
			}
			original := turntableCalendarRestoreSnapshot(t)
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				if err := RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after); err != nil {
					return err
				}
				return errors.New("subsequent migration fails")
			}); err == nil {
				t.Fatal("expected caller rollback")
			}
			if !reflect.DeepEqual(original, turntableCalendarRestoreSnapshot(t)) {
				t.Fatal("rollback changed persistent state")
			}
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after) }); err != nil {
				t.Fatal(err)
			}
			expected := original[0].([]db.GameplayPool)
			expected[0].StartAt, expected[0].EndAt, expected[0].RewardEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
			if !reflect.DeepEqual(original, turntableCalendarRestoreSnapshot(t)) {
				t.Fatal("migration modified non-window columns, byte payloads or historical rows")
			}
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after) }); err == nil {
				t.Fatal("stale before window accepted")
			}
		})
	}
}

func TestTurntableCalendarRestoreRejectsAnomalies(t *testing.T) {
	for _, scenario := range []string{"bare-db", "old-window", "invalid-target", "daily-quota", "negative-quota", "settled-global", "settled-personal", "invalid-global-stock", "unknown-stock-id", "duplicate-stock-key", "overused-stock", "invalid-pity", "unknown-pity-field", "invalid-free-day", "negative-score", "missing-rules", "missing-reward", "prior-ranks"} {
		t.Run(scenario, func(t *testing.T) {
			pool, before, after := turntableCalendarRestoreFixture(t)
			var err error
			switch scenario {
			case "old-window":
				before.EndAt++
			case "invalid-target":
				after.EndAt = after.StartAt
			case "daily-quota":
				err = db.GetIns().Model(&pool).Update("free_per_day", 1).Error
			case "negative-quota":
				err = db.GetIns().Model(&pool).Update("free_per_day", -2).Error
			case "settled-global":
				err = db.GetIns().Model(&db.TurntableGlobal{}).Where("pool_id=?", pool.ID).Update("settled", true).Error
			case "settled-personal":
				err = db.GetIns().Model(&db.TurntablePersonal{}).Where("pool_id=?", pool.ID).Update("settled", true).Error
			case "invalid-global-stock":
				err = db.GetIns().Model(&db.TurntableGlobal{}).Where("pool_id=?", pool.ID).Update("used_stock", []byte(`null`)).Error
			case "unknown-stock-id":
				err = db.GetIns().Model(&db.TurntableGlobal{}).Where("pool_id=?", pool.ID).Update("used_stock", []byte(`{"99":1}`)).Error
			case "duplicate-stock-key":
				err = db.GetIns().Model(&db.TurntableGlobal{}).Where("pool_id=?", pool.ID).Update("used_stock", []byte(`{"0":1,"0":2}`)).Error
			case "overused-stock":
				err = db.GetIns().Model(&db.TurntablePersonal{}).Where("pool_id=?", pool.ID).Update("used_stock", []byte(`{"0":6}`)).Error
			case "invalid-pity":
				err = db.GetIns().Model(&db.TurntableState{}).Where("pool_id=?", pool.ID).Update("prize_pity_state", `{"0":{"key":"k","since":-1}}`).Error
			case "unknown-pity-field":
				err = db.GetIns().Model(&db.TurntableState{}).Where("pool_id=?", pool.ID).Update("prize_pity_state", `{"0":{"key":"k","since":1,"extra":1}}`).Error
			case "invalid-free-day":
				err = db.GetIns().Model(&db.TurntableState{}).Where("pool_id=?", pool.ID).Update("free_day", "2026-99-99").Error
			case "negative-score":
				err = db.GetIns().Model(&db.TurntableState{}).Where("pool_id=?", pool.ID).Update("score", -1).Error
			case "missing-rules":
				err = db.GetIns().Where("pool_id=?", pool.ID).Delete(&db.TurntableRules{}).Error
			case "missing-reward":
				err = db.GetIns().Where("pool_id=? AND id=8", pool.ID).Delete(&db.GameplayPoolReward{}).Error
			case "prior-ranks":
				err = db.GetIns().Model(&db.TurntableGlobal{}).Where("pool_id=?", pool.ID).Update("final_ranks", []byte(`[{"rank":1}]`)).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			original := turntableCalendarRestoreSnapshot(t)
			if scenario == "bare-db" {
				err = RestoreActivityCalendarTurntableWindow(db.GetIns(), pool.ID, before, after)
			} else {
				err = db.GetIns().Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after) })
			}
			if err == nil {
				t.Fatal("expected refusal")
			}
			if !reflect.DeepEqual(original, turntableCalendarRestoreSnapshot(t)) {
				t.Fatal("rejected migration changed state")
			}
		})
	}
}

func TestTurntableCalendarRestoreMissingGlobalNeverCreatesState(t *testing.T) {
	for _, emptyPersonal := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-personal", true: "lazy-personal"}[emptyPersonal], func(t *testing.T) {
			pool, before, after := turntableCalendarRestoreFixture(t)
			if err := db.GetIns().Where("pool_id=?", pool.ID).Delete(&db.TurntableGlobal{}).Error; err != nil {
				t.Fatal(err)
			}
			if emptyPersonal {
				if err := db.GetIns().Where("pool_id=?", pool.ID).Delete(&db.TurntablePersonal{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			original := turntableCalendarRestoreSnapshot(t)
			rollback := errors.New("later migration failed")
			err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				if err := RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after); err != nil {
					return err
				}
				return rollback
			})
			if !errors.Is(err, rollback) || !reflect.DeepEqual(original, turntableCalendarRestoreSnapshot(t)) {
				t.Fatal("missing-global rollback changed state", err)
			}
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarTurntableWindow(tx, pool.ID, before, after) }); err != nil {
				t.Fatal(err)
			}
			expected := original[0].([]db.GameplayPool)
			expected[0].StartAt, expected[0].EndAt, expected[0].RewardEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
			if !reflect.DeepEqual(original, turntableCalendarRestoreSnapshot(t)) {
				t.Fatal("migration created shared/personal state or changed historical payloads")
			}
		})
	}
}

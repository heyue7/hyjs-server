package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func kingCalendarRestoreFixture(t *testing.T, frozen bool) (*GameService, db.GameplayPool, FrozenJSONWindow, FrozenJSONWindow) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "king-calendar.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	if err := db.GetIns().AutoMigrate(&db.GameplayPool{}, &db.GameplayPoolReward{}, &db.KingRecruitActivitySnapshot{}, &db.KingRecruitState{}, &db.RedP2Receipt{}, &db.KingRecruitSelection{}, &db.KingRecruitRules{}); err != nil {
		t.Fatal(err)
	}
	s := &GameService{Tables: map[string][]map[string]string{"GameConfigure": {{"StaticName": "WarriorRecruitCostType3", "NameValue": "diamond,300"}, {"StaticName": "WarriorRecruitCostType3NumType2", "NameValue": "diamond,2800"}, {"StaticName": "BuyCoinCountOfRecruitType3", "NameValue": "10000"}}, "Hero": {{"id": "hero-fixture", "heroClass": "0", "quality": "23", "space": "1"}}}}
	before, after := FrozenJSONWindow{1000, 2000, 3000}, FrozenJSONWindow{4000, 5000, 6000}
	pool := db.GameplayPool{ID: constUtil.AdminKingRecruitPrefix + "calendar", Name: "preserved", Source: "isolated fixture", Enabled: true, MinLevel: 20, SingleCost: "diamond,300", TenCost: "diamond,2800", CoinPerDraw: 10000, FreeSeconds: 172800, FreePerDay: -1, StartAt: 1000, EndAt: 2000, RewardEndAt: 3000}
	rules := db.KingRecruitRules{ID: 1, DailyLimit: 50, LuckLimit: 500, LuckPerDraw: 10, RotationMillis: 172800000, EpochMillis: 100, Source: "frozen rules", WheelPool: "historic-wheel"}
	payload := adminJSON(adminKingRecruitSource{World: 1, Pool: pool, Rules: rules})
	var fields map[string]json.RawMessage
	json.Unmarshal(payload, &fields)
	fields["unknown"] = []byte(`{"large":9007199254740993,"label":"keep"}`)
	var poolFields map[string]json.RawMessage
	json.Unmarshal(fields["pool"], &poolFields)
	poolFields["historicalExtension"] = []byte(`{"price":7,"window":"retained"}`)
	fields["pool"], _ = json.Marshal(poolFields)
	payload, _ = json.Marshal(fields)
	for _, row := range []any{&pool, &db.GameplayPoolReward{PoolID: pool.ID, ID: 1, ResourceID: "hero-fixture", Amount: 1, Weight: 100, Guaranteed: true}, &db.KingRecruitActivitySnapshot{PoolID: pool.ID, Payload: payload, Frozen: frozen}, &rules, &db.KingRecruitState{PlayerID: 7, Day: "2026-10-27", Draws: 19, Luck: 230}, &db.KingRecruitSelection{PlayerID: 7, PoolID: pool.ID}, &db.RedP2Receipt{PlayerID: 7, Operation: "king-recruit", Timestamp: 123, Request: []byte{1, 2}, Response: []byte{3, 4}}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return s, pool, before, after
}

func kingCalendarRestoreSnapshot(t *testing.T) []any {
	t.Helper()
	var pools []db.GameplayPool
	var rewards []db.GameplayPoolReward
	var snapshots []db.KingRecruitActivitySnapshot
	var rules []db.KingRecruitRules
	var states []db.KingRecruitState
	var selections []db.KingRecruitSelection
	var receipts []db.RedP2Receipt
	for _, target := range []any{&pools, &rewards, &snapshots, &rules, &states, &selections, &receipts} {
		if err := db.GetIns().Find(target).Error; err != nil {
			t.Fatal(err)
		}
	}
	return []any{pools, rewards, snapshots, rules, states, selections, receipts}
}

func TestKingRecruitCalendarRestorePreservesFrozenAndHistory(t *testing.T) {
	for _, frozen := range []bool{false, true} {
		t.Run(map[bool]string{false: "unfrozen", true: "frozen"}[frozen], func(t *testing.T) {
			s, pool, before, after := kingCalendarRestoreFixture(t, frozen)
			original := kingCalendarRestoreSnapshot(t)
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return s.RestoreActivityCalendarKingRecruitWindow(tx, pool.ID, before, after) }); err != nil {
				t.Fatal(err)
			}
			updated := kingCalendarRestoreSnapshot(t)
			expectedPool := original[0].([]db.GameplayPool)
			expectedPool[0].StartAt, expectedPool[0].EndAt, expectedPool[0].RewardEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
			oldSnapshot := original[2].([]db.KingRecruitActivitySnapshot)
			newSnapshot := updated[2].([]db.KingRecruitActivitySnapshot)
			var oldFields, newFields map[string]json.RawMessage
			json.Unmarshal(oldSnapshot[0].Payload, &oldFields)
			json.Unmarshal(newSnapshot[0].Payload, &newFields)
			var oldPool, newPool map[string]json.RawMessage
			json.Unmarshal(oldFields["pool"], &oldPool)
			json.Unmarshal(newFields["pool"], &newPool)
			for _, key := range []string{"startAt", "endAt", "rewardEndAt"} {
				delete(oldPool, key)
				delete(newPool, key)
			}
			delete(oldFields, "pool")
			delete(newFields, "pool")
			if !reflect.DeepEqual(oldFields, newFields) || !reflect.DeepEqual(oldPool, newPool) {
				t.Fatal("non-calendar frozen JSON changed")
			}
			oldSnapshot[0].Payload = newSnapshot[0].Payload
			if !reflect.DeepEqual(original, updated) {
				t.Fatal("non-calendar rows or Frozen/history bytes changed")
			}
			var source adminKingRecruitSource
			if json.Unmarshal(newSnapshot[0].Payload, &source) != nil || (FrozenJSONWindow{source.Pool.StartAt, source.Pool.EndAt, source.Pool.RewardEndAt}) != after {
				t.Fatal("nested window not migrated")
			}
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return s.RestoreActivityCalendarKingRecruitWindow(tx, pool.ID, before, after) }); err == nil {
				t.Fatal("stale old window accepted")
			}
		})
	}
}

func TestKingRecruitCalendarRestoreRejectsAndRollsBack(t *testing.T) {
	for _, scenario := range []string{"bare-db", "rollback", "old-window", "invalid-target", "snapshot-window", "snapshot-id", "missing-snapshot", "world", "world-type", "rules", "reward-world", "duplicate-json"} {
		t.Run(scenario, func(t *testing.T) {
			s, pool, before, after := kingCalendarRestoreFixture(t, true)
			var snapshot db.KingRecruitActivitySnapshot
			if err := db.GetIns().Where("pool_id=?", pool.ID).Take(&snapshot).Error; err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			json.Unmarshal(snapshot.Payload, &fields)
			var nested map[string]json.RawMessage
			json.Unmarshal(fields["pool"], &nested)
			switch scenario {
			case "old-window":
				before.EndAt++
			case "invalid-target":
				after.EndAt = after.StartAt
			case "snapshot-window":
				nested["endAt"] = adminJSON(int64(1999))
			case "snapshot-id":
				nested["id"] = adminJSON("another-pool")
			case "world":
				fields["world"] = adminJSON(int64(4))
			case "world-type":
				fields["world"] = []byte(`"1"`)
			case "rules":
				var rules map[string]json.RawMessage
				json.Unmarshal(fields["rules"], &rules)
				rules["dailyLimit"] = adminJSON(51)
				fields["rules"], _ = json.Marshal(rules)
			case "reward-world":
				fields["world"] = adminJSON(int64(2))
			}
			fields["pool"], _ = json.Marshal(nested)
			payload, _ := json.Marshal(fields)
			if scenario == "duplicate-json" {
				payload = []byte(`{"pool":{},"pool":{}}`)
			}
			if err := db.GetIns().Model(&snapshot).Update("payload", payload).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "missing-snapshot" {
				if err := db.GetIns().Where("pool_id=?", pool.ID).Delete(&db.KingRecruitActivitySnapshot{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			original := kingCalendarRestoreSnapshot(t)
			var err error
			if scenario == "bare-db" {
				err = s.RestoreActivityCalendarKingRecruitWindow(db.GetIns(), pool.ID, before, after)
			} else {
				err = db.GetIns().Transaction(func(tx *gorm.DB) error {
					if err := s.RestoreActivityCalendarKingRecruitWindow(tx, pool.ID, before, after); err != nil {
						return err
					}
					if scenario == "rollback" {
						return errors.New("later domain rejected")
					}
					return nil
				})
			}
			if err == nil {
				t.Fatal("expected refusal")
			}
			if !reflect.DeepEqual(original, kingCalendarRestoreSnapshot(t)) {
				t.Fatal("failed restore changed state")
			}
		})
	}
}

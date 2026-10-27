package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func openingRestoreFixture(t *testing.T) (db.AdminGameServer, []db.PlayerOpeningCalendar, db.OpenServerActivityRule, int64, FrozenJSONWindow, FrozenJSONWindow) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "opening-window.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	if err := db.GetIns().AutoMigrate(&db.AdminGameServer{}, &db.PlayerOpeningCalendar{}, &db.OpenServerActivityRule{}, &db.OpenServerActivityState{}, &db.OpenServerActivityCounter{}, &db.OpenServerActivityReceipt{}, &db.GameAccount{}); err != nil {
		t.Fatal(err)
	}
	start := openingCarryTestDay("2026-10-01")
	next := openingCarryTestDay("2026-10-27")
	day := int64(86400000)
	realm := db.AdminGameServer{ID: 1, Name: "retained", Enabled: true, RuntimeBound: true, OpenAt: start + 1234, Revision: 17}
	anchor := db.PlayerOpeningCalendar{PlayerID: 7, StartAt: start + 2*day, CreatedAt: start + 12345, Source: constUtil.PlayerOpeningCalendarSource}
	before, after := FrozenJSONWindow{start, start + 14*day, start + 16*day}, FrozenJSONWindow{next, next + 14*day, next + 16*day}
	opening := db.OpenServerActivityRule{ID: constUtil.OpenServerActivityID, Enabled: true, StartAt: before.StartAt, EndAt: before.EndAt, RewardEndAt: before.ClaimEndAt, Source: "frozen template rules"}
	player := "{}"
	birth := start - 12345
	for _, row := range []any{&realm, &anchor, &opening, &db.Profile{Id: 1, Uid: "protected", ServerID: 1, Player: &player, Game: &player, Session: []byte{1}, CreatedAt: &birth}, &db.Profile{Id: 7, Uid: "owner", ServerID: 1, Player: &player, Game: &player, Session: []byte{2}, CreatedAt: &birth}, &db.Profile{Id: 8, Uid: "missing-anchor", ServerID: 1, Player: &player, Game: &player, Session: []byte{3}, CreatedAt: &birth}, &db.Profile{Id: 9, Uid: "other-realm", ServerID: 2, Player: &player, Game: &player, Session: []byte{4}, CreatedAt: &birth}, &db.OpenServerActivityState{PlayerID: 7, TaskID: 1, Achieved: true, Progress: 3, Claims: 1}, &db.OpenServerActivityCounter{PlayerID: 7, CounterKey: "login:2026-10-01", Value: 1}, &db.OpenServerActivityReceipt{PlayerID: 7, Timestamp: 123, Request: []byte{1, 2}, Response: []byte{3, 4}}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return realm, []db.PlayerOpeningCalendar{anchor}, opening, next, before, after
}
func openingRestoreSnapshot(t *testing.T) []any {
	t.Helper()
	var realms []db.AdminGameServer
	var anchors []db.PlayerOpeningCalendar
	var openings []db.OpenServerActivityRule
	var profiles []db.Profile
	var states []db.OpenServerActivityState
	var counters []db.OpenServerActivityCounter
	var receipts []db.OpenServerActivityReceipt
	for _, row := range []any{&realms, &anchors, &openings, &profiles, &states, &counters, &receipts} {
		if err := db.GetIns().Find(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return []any{realms, anchors, openings, profiles, states, counters, receipts}
}

func TestActivityCalendarOpeningRestorePreservesHistoryAndMissingAnchor(t *testing.T) {
	realm, anchors, opening, next, before, after := openingRestoreFixture(t)
	original := openingRestoreSnapshot(t)
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		return RestoreActivityCalendarOpeningWindows(tx, realm, anchors, opening, next, before, after)
	}); err != nil {
		t.Fatal(err)
	}
	expectedRealm := original[0].([]db.AdminGameServer)
	expectedRealm[0].OpenAt = next
	expectedRealm[0].Revision++
	expectedAnchors := original[1].([]db.PlayerOpeningCalendar)
	expectedAnchors[0].StartAt = next
	expectedOpening := original[2].([]db.OpenServerActivityRule)
	expectedOpening[0].StartAt, expectedOpening[0].EndAt, expectedOpening[0].RewardEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
	if !reflect.DeepEqual(original, openingRestoreSnapshot(t)) {
		t.Fatal("non-calendar fields, birthdays or historical ledger changed")
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		return RestoreActivityCalendarOpeningWindows(tx, realm, anchors, opening, next, before, after)
	}); err == nil {
		t.Fatal("stale version accepted")
	}
}

func TestActivityCalendarOpeningRestoreRejectsAndRollsBack(t *testing.T) {
	for _, scenario := range []string{"bare-db", "rollback", "realm-zero", "next-not-midnight", "duration", "grace-change", "duplicate-owner", "invalid-source", "anchor-zero", "missing-anchor", "new-anchor", "anchor-date-drift", "anchor-source-drift", "realm-version", "realm-name", "template-source", "template-date", "foreign-owner", "missing-account"} {
		t.Run(scenario, func(t *testing.T) {
			realm, anchors, opening, next, before, after := openingRestoreFixture(t)
			var err error
			switch scenario {
			case "realm-zero":
				realm.OpenAt = 0
			case "next-not-midnight":
				next++
			case "duration":
				after.EndAt++
			case "grace-change":
				after.ClaimEndAt++
			case "duplicate-owner":
				anchors = append(anchors, anchors[0])
			case "invalid-source":
				anchors[0].Source = "unknown"
			case "anchor-zero":
				anchors[0].StartAt = 0
			case "missing-anchor":
				err = db.GetIns().Where("player_id=7").Delete(&db.PlayerOpeningCalendar{}).Error
			case "new-anchor":
				row := anchors[0]
				row.PlayerID = 8
				err = db.GetIns().Create(&row).Error
			case "anchor-date-drift":
				err = db.GetIns().Model(&db.PlayerOpeningCalendar{}).Where("player_id=7").Update("start_at", anchors[0].StartAt+86400000).Error
			case "anchor-source-drift":
				err = db.GetIns().Model(&db.PlayerOpeningCalendar{}).Where("player_id=7").Update("source", "changed").Error
			case "realm-version":
				err = db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", realm.ID).Update("revision", realm.Revision+1).Error
			case "realm-name":
				err = db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", realm.ID).Update("name", "changed").Error
			case "template-source":
				err = db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", opening.ID).Update("source", "changed").Error
			case "template-date":
				err = db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", opening.ID).Update("end_at", opening.EndAt+1).Error
			case "foreign-owner":
				anchors[0].PlayerID = 9
				row := anchors[0]
				err = db.GetIns().Create(&row).Error
			case "protected-anchor":
				row := anchors[0]
				row.PlayerID = 1
				anchors = append(anchors, row)
				err = db.GetIns().Create(&row).Error
			case "missing-account":
				err = db.GetIns().Model(&db.Profile{}).Where("id=7").Update("account_id", 999).Error
			case "protected-account":
				account := db.GameAccount{ID: 77, Username: "protected-global", PasswordSalt: []byte{1}, PasswordHash: []byte{2}, Enabled: true, Protected: true, CreatedAt: next}
				if err = db.GetIns().Create(&account).Error; err == nil {
					err = db.GetIns().Model(&db.Profile{}).Where("id=7").Update("account_id", account.ID).Error
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			original := openingRestoreSnapshot(t)
			if scenario == "bare-db" {
				err = RestoreActivityCalendarOpeningWindows(db.GetIns(), realm, anchors, opening, next, before, after)
			} else {
				err = db.GetIns().Transaction(func(tx *gorm.DB) error {
					if err := RestoreActivityCalendarOpeningWindows(tx, realm, anchors, opening, next, before, after); err != nil {
						return err
					}
					if scenario == "rollback" {
						return errors.New("later operation rejected")
					}
					return nil
				})
			}
			if err == nil {
				t.Fatal("expected refusal")
			}
			if !reflect.DeepEqual(original, openingRestoreSnapshot(t)) {
				t.Fatal("failed migration modified state")
			}
		})
	}
}

func TestActivityCalendarOpeningRestoreSkipsProtectedAndPrivateAnchors(t *testing.T) {
	realm, anchors, opening, next, before, after := openingRestoreFixture(t)
	if err := db.GetIns().AutoMigrate(&db.LocalPlayActivityBinding{}); err != nil {
		t.Fatal(err)
	}
	protected := db.PlayerOpeningCalendar{PlayerID: 1, StartAt: before.StartAt, CreatedAt: before.StartAt + 1, Source: constUtil.PlayerOpeningCalendarSource}
	if err := db.GetIns().Create(&protected).Error; err != nil {
		t.Fatal(err)
	}
	anchors = append(anchors, protected)
	if err := db.GetIns().Create(&db.LocalPlayActivityBinding{PlayerID: 7, ActivityID: constUtil.OpenServerActivityID, CycleID: "private-kept"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		return RestoreActivityCalendarOpeningWindows(tx, realm, anchors, opening, next, before, after)
	}); err != nil {
		t.Fatal(err)
	}
	for _, old := range anchors {
		var current db.PlayerOpeningCalendar
		if err := db.GetIns().First(&current, "player_id=?", old.PlayerID).Error; err != nil || !reflect.DeepEqual(old, current) {
			t.Fatal("excluded anchor changed", err)
		}
	}
}

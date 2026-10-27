package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/db"
)

func activityScheduleRestoreFixture(t *testing.T, mode string) (*ActivityCalendarRestorePlan, []db.AdminActivityOccurrence, []db.AdminCharacterActivityInstance) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "schedule-restore.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}); err != nil {
		t.Fatal(err)
	}
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
	source := db.AdminActivityOccurrence{ID: "occ-test", ServerID: 1, Timing: "character-day", StartDay: 1, DurationDays: duration, ClaimDays: 1, TemplateID: "template", TemplateVersion: "original-template", Name: "source", Mode: mode, ConsumerID: "character-plan:source", ConsumerKey: "source", StartAt: start, EndAt: start + duration*day, ClaimEndAt: start + (duration+1)*day, Enabled: true, Revision: 9, FirstPublishedAt: 123, FormJSON: form(start+day, duration, "source-private")}
	instance := db.AdminCharacterActivityInstance{PlayerID: 7, OccurrenceID: source.ID, ServerID: 1, ConsumerID: "period:own", ConsumerKey: "own", Mode: mode, StartAt: start + 4*day, EndAt: start + (4+duration)*day, ClaimEndAt: start + (5+duration)*day, SourceVersion: "accepted-older-version", FormJSON: form(start+4*day, duration, "frozen-private")}
	if mode == "opening-14" {
		instance.ConsumerID = "opening:opening_character_1_7"
		instance.ConsumerKey = "opening_character_1_7"
		instance.FormJSON = form(start-30*day, duration, "old-source-frozen")
	}
	for _, row := range []any{&source, &instance} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Exact persisted images include defaults and every hidden identity field.
	if err := db.GetIns().Where("id=?", source.ID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("player_id=? AND occurrence_id=?", instance.PlayerID, instance.OccurrenceID).Take(&instance).Error; err != nil {
		t.Fatal(err)
	}
	next := openingCarryTestDay("2026-10-27")
	after := [3]int64{next, next + nextDuration*day, next + (nextDuration+1)*day}
	plan := &ActivityCalendarRestorePlan{ServerID: 1, Windows: []ActivityCalendarRestoreWindow{{OccurrenceID: source.ID, ConsumerID: source.ConsumerID, Mode: source.Mode, SourceVersion: adminVersion(source), Before: [3]int64{source.StartAt, source.EndAt, source.ClaimEndAt}, After: after, StartDay: 1, DurationDays: nextDuration, ClaimDays: 1}, {OccurrenceID: source.ID, PlayerID: 7, ConsumerID: instance.ConsumerID, Mode: instance.Mode, SourceVersion: instance.SourceVersion, Before: [3]int64{instance.StartAt, instance.EndAt, instance.ClaimEndAt}, After: after, StartDay: 1, DurationDays: nextDuration, ClaimDays: 1}}}
	return plan, []db.AdminActivityOccurrence{source}, []db.AdminCharacterActivityInstance{instance}
}

func activityScheduleRestoreRead(t *testing.T) ([]db.AdminActivityOccurrence, []db.AdminCharacterActivityInstance) {
	t.Helper()
	var sources []db.AdminActivityOccurrence
	var issued []db.AdminCharacterActivityInstance
	if err := db.GetIns().Order("id").Find(&sources).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Order("player_id,occurrence_id").Find(&issued).Error; err != nil {
		t.Fatal(err)
	}
	return sources, issued
}
func activityScheduleRestoreNonDates(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"startAt", "endAt", "claimEndAt"} {
		delete(out, key)
	}
	return out
}

func TestActivityCalendarScheduleRestorePreservesHistoricalSource(t *testing.T) {
	for _, mode := range []string{"recharge-total", "opening-14"} {
		t.Run(mode, func(t *testing.T) {
			plan, sources, issued := activityScheduleRestoreFixture(t, mode)
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarScheduleWindows(tx, plan, sources, issued) }); err != nil {
				t.Fatal(err)
			}
			afterSources, afterIssued := activityScheduleRestoreRead(t)
			for i := range sources {
				if !reflect.DeepEqual(activityScheduleRestoreNonDates(t, sources[i].FormJSON), activityScheduleRestoreNonDates(t, afterSources[i].FormJSON)) {
					t.Fatal("source rules or extra fields changed")
				}
				expected := sources[i]
				w := plan.Windows[0]
				expected.StartAt, expected.EndAt, expected.ClaimEndAt = w.After[0], w.After[1], w.After[2]
				expected.DurationDays = w.DurationDays
				expected.Revision++
				expected.FormJSON = afterSources[i].FormJSON
				if !reflect.DeepEqual(expected, afterSources[i]) {
					t.Fatal("source non-calendar fields changed")
				}
			}
			if !reflect.DeepEqual(activityScheduleRestoreNonDates(t, issued[0].FormJSON), activityScheduleRestoreNonDates(t, afterIssued[0].FormJSON)) {
				t.Fatal("historical frozen terms replaced by current source")
			}
			expected := issued[0]
			w := plan.Windows[1]
			expected.StartAt, expected.EndAt, expected.ClaimEndAt = w.After[0], w.After[1], w.After[2]
			expected.FormJSON = afterIssued[0].FormJSON
			if !reflect.DeepEqual(expected, afterIssued[0]) || afterIssued[0].SourceVersion != "accepted-older-version" {
				t.Fatal("acceptance provenance changed")
			}
			jsonWindow, err := activityScheduleJSONWindow(afterIssued[0].FormJSON, mode)
			if err != nil || jsonWindow != (FrozenJSONWindow{w.After[0], w.After[1], w.After[2]}) {
				t.Fatal("issued JSON target", err)
			}
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return RestoreActivityCalendarScheduleWindows(tx, plan, sources, issued) }); err == nil {
				t.Fatal("stale preimage accepted without outer request receipt")
			}
		})
	}
}

func TestActivityCalendarScheduleRestoreConflictsAndRollback(t *testing.T) {
	for _, scenario := range []string{"bare-db", "rollback", "source-form-hidden", "source-consumer-key", "source-version", "source-revision", "source-disabled", "issued-form", "issued-version", "issued-consumer-key", "issued-mode", "new-issued", "missing-snapshot", "plan-window", "unknown-format", "duplicate-json", "ordinary-issued-time"} {
		t.Run(scenario, func(t *testing.T) {
			plan, sources, issued := activityScheduleRestoreFixture(t, "recharge-total")
			var err error
			switch scenario {
			case "source-form-hidden":
				err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", sources[0].ID).Update("form_json", append([]byte(" "), sources[0].FormJSON...)).Error
			case "source-consumer-key":
				err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", sources[0].ID).Update("consumer_key", "changed").Error
			case "source-version":
				plan.Windows[0].SourceVersion = "incorrect"
			case "source-revision":
				err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", sources[0].ID).Update("revision", 10).Error
			case "source-disabled":
				err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", sources[0].ID).Update("enabled", false).Error
			case "issued-form":
				err = db.GetIns().Model(&db.AdminCharacterActivityInstance{}).Where("player_id=7").Update("form_json", append([]byte(" "), issued[0].FormJSON...)).Error
			case "issued-version":
				err = db.GetIns().Model(&db.AdminCharacterActivityInstance{}).Where("player_id=7").Update("source_version", "drifted").Error
			case "issued-consumer-key":
				err = db.GetIns().Model(&db.AdminCharacterActivityInstance{}).Where("player_id=7").Update("consumer_key", "other").Error
			case "issued-mode":
				err = db.GetIns().Model(&db.AdminCharacterActivityInstance{}).Where("player_id=7").Update("mode", "opening-14").Error
			case "new-issued":
				row := issued[0]
				row.PlayerID = 8
				row.ConsumerID = "period:other"
				row.ConsumerKey = "other"
				err = db.GetIns().Create(&row).Error
			case "missing-snapshot":
				issued = nil
			case "plan-window":
				plan.Windows[1].Before[1]++
			case "unknown-format":
				sources[0].FormJSON = []byte(`{"mode":"recharge-total","start_at":1}`)
				err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", sources[0].ID).Update("form_json", sources[0].FormJSON).Error
			case "duplicate-json":
				sources[0].FormJSON = []byte(`{"mode":"recharge-total","mode":"recharge-total","startAt":1,"endAt":2,"claimEndAt":3}`)
				err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", sources[0].ID).Update("form_json", sources[0].FormJSON).Error
			case "ordinary-issued-time":
				issued[0].FormJSON = sources[0].FormJSON
				err = db.GetIns().Model(&db.AdminCharacterActivityInstance{}).Where("player_id=7").Update("form_json", issued[0].FormJSON).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			beforeSources, beforeIssued := activityScheduleRestoreRead(t)
			if scenario == "bare-db" {
				err = RestoreActivityCalendarScheduleWindows(db.GetIns(), plan, sources, issued)
			} else {
				err = db.GetIns().Transaction(func(tx *gorm.DB) error {
					if e := RestoreActivityCalendarScheduleWindows(tx, plan, sources, issued); e != nil {
						return e
					}
					if scenario == "rollback" {
						return errors.New("subsequent consumer migration fails")
					}
					return nil
				})
			}
			if err == nil {
				t.Fatal("expected conflict or rollback")
			}
			afterSources, afterIssued := activityScheduleRestoreRead(t)
			if !reflect.DeepEqual(beforeSources, afterSources) || !reflect.DeepEqual(beforeIssued, afterIssued) {
				t.Fatal("failure changed schedule or frozen state")
			}
		})
	}
}

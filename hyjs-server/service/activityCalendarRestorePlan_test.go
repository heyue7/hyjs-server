package service

import (
	"strings"
	"testing"
	"time"

	"hyjs-server/db"
	"hyjs-server/model"
)

func TestActivityCalendarRestoreRejectsInvalidMonthlyDay(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*3600)
	old := time.Date(2026, 10, 1, 0, 0, 0, 0, zone)
	start := time.Date(2026, 10, 31, 0, 0, 0, 0, zone)
	row := db.AdminActivityOccurrence{ID: "month-end", Mode: "recharge-total", StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 0, 1).UnixMilli(), ClaimEndAt: start.AddDate(0, 0, 2).UnixMilli(), FormJSON: adminJSON(model.AdminActivityForm{Name: "月度盛典"})}
	plan := activityCalendarRestorePlan(db.AdminGameServer{ID: 1, OpenAt: old.UnixMilli()}, db.ServerGameClock{}, db.AdminActivityScheduleRevision{}, []db.AdminActivityOccurrence{row}, nil, nil, time.Date(2026, 11, 1, 12, 0, 0, 0, zone))
	if len(plan.Windows) != 0 || len(plan.Blockers) != 1 || !strings.Contains(plan.Blockers[0], "does not exist") {
		t.Fatalf("invalid monthly date was not blocked: %+v", plan)
	}
}

func TestActivityCalendarRestorePreservesIdentityAndCalendarBoundaries(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*3600)
	old := time.Date(2026, 10, 11, 0, 0, 0, 0, zone)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, zone)
	rows := []db.AdminActivityOccurrence{}
	for i, start := range []int64{1, 15, 29} {
		rows = append(rows, db.AdminActivityOccurrence{ID: string(rune('a' + i)), ConsumerID: string(rune('k' + i)), Timing: "character-day", StartDay: start, DurationDays: 14, Mode: "turntable", FormJSON: adminJSON(model.AdminActivityForm{Name: "幸运大转盘"}), Enabled: true})
	}
	rows = append(rows, db.AdminActivityOccurrence{ID: "recharge", ConsumerID: "period:old", Timing: "character-day", StartDay: 1, DurationDays: 14, Mode: "recharge-total", FormJSON: adminJSON(model.AdminActivityForm{Name: "启程庆典"}), Enabled: true})
	issued := []db.AdminCharacterActivityInstance{{PlayerID: 39, OccurrenceID: "a", ConsumerID: "turntable:owned", SourceVersion: "frozen", StartAt: 1, EndAt: 2, ClaimEndAt: 3, FormJSON: []byte(`{"rewards":"keep"}`)}}
	plan := activityCalendarRestorePlan(db.AdminGameServer{ID: 1, OpenAt: old.UnixMilli()}, db.ServerGameClock{OffsetDays: -10, Revision: 7}, db.AdminActivityScheduleRevision{Revision: 9}, rows, issued, nil, now)
	if !plan.ReadOnly || plan.ClockDeltaDays != 10 || len(plan.Windows) != 5 {
		t.Fatalf("bad plan %+v", plan)
	}
	for i, want := range [][2]int64{{1, 7}, {8, 14}, {22, 14}, {1, 3}} {
		got := plan.Windows[i]
		if got.StartDay != want[0] || got.DurationDays != want[1] {
			t.Fatalf("wrong window %+v", got)
		}
	}
	if plan.Windows[4].ConsumerID != "turntable:owned" || plan.Windows[4].PlayerID != 39 || len(plan.Blockers) != 0 {
		t.Fatal("issued identity lost or unexpected blocker")
	}
	before := plan.Digest
	issued[0].FormJSON = []byte(`{"rewards":"changed"}`)
	next := activityCalendarRestorePlan(db.AdminGameServer{ID: 1, OpenAt: old.UnixMilli()}, db.ServerGameClock{OffsetDays: -10, Revision: 7}, db.AdminActivityScheduleRevision{Revision: 9}, rows, issued, nil, now)
	if before == next.Digest {
		t.Fatal("payload drift not captured")
	}
}

func TestActivityCalendarRestorePreviewSQLiteDoesNotWrite(t *testing.T) {
	f, _ := growth75DeploymentFixture(t, 0)
	if err := db.GetIns().AutoMigrate(&db.PlayerOpeningCalendar{}, &db.AdminGameServer{}, &db.AdminActivityScheduleRevision{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.AdminGameServer{ID: 1, OpenAt: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC).UnixMilli(), Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.AdminActivityScheduleRevision{ServerID: 1, Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	plan, err := f.s.PreviewActivityCalendarRestore(f.ctx, time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ReadOnly || len(plan.Blockers) < 2 {
		t.Fatalf("missing maintenance/schema blockers %+v", plan)
	}
	var after int64
	if err = db.GetIns().Model(&db.AdminAudit{}).Count(&after).Error; err != nil || before != after {
		t.Fatal("preview wrote audit", err)
	}
}

func TestActivityCalendarRestoreDependenciesKeepHistoricalTierIdentity(t *testing.T) {
	f, _ := growth75DeploymentFixture(t, 0)
	cfg := db.PeriodInstance{ID: "calendar-restore-test", RewardSourceID: "original-tier-owner", Goal: 1, Rewards: "coin,1", Milestones: adminJSON([]map[string]any{{"goal": 1, "rewards": "coin,1"}, {"goal": 2, "rewards": "coin,2"}})}
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.PeriodProgress{InstanceID: cfg.ID, PlayerID: f.id, Snapshot: adminJSON(cfg), Value: 2, LastDay: "2026-10-11", RewardID: 9123}).Error; err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"activity:original-tier-owner", "activity:original-tier-owner:tier:2", "activity:original-tier-owner:tier:20-other", "rank:" + cfg.ID, "rank:" + cfg.ID + "-other"} {
		if err := db.GetIns().Create(&db.RewardDelivery{PlayerID: f.id, SourceKey: source, Rewards: "coin,1", ClaimedAt: 100, Result: []byte("preserve")}).Error; err != nil {
			t.Fatal(err)
		}
	}
	plan := &ActivityCalendarRestorePlan{ReadOnly: true, Windows: []ActivityCalendarRestoreWindow{{ConsumerID: "period:" + cfg.ID}}, Blockers: []string{}}
	if err := activityCalendarRestoreDependencies(db.GetIns(), plan); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, entry := range plan.Dependencies {
		counts[entry.Table] = entry.Rows
	}
	if counts["p1_period_instances"] != 1 || counts["p1_period_progress"] != 1 || counts["p1_reward_deliveries"] != 3 {
		t.Fatalf("wrong exact tier-source mapping %+v", counts)
	}
	var after db.PeriodProgress
	if err := db.GetIns().First(&after, "instance_id=? AND player_id=?", cfg.ID, f.id).Error; err != nil || after.LastDay != "2026-10-11" || after.Value != 2 || after.RewardID != 9123 {
		t.Fatal("dependency inspection altered progress", err)
	}
	var delivery db.RewardDelivery
	if err := db.GetIns().First(&delivery, "player_id=? AND source_key=?", f.id, "activity:original-tier-owner:tier:2").Error; err != nil || delivery.ClaimedAt != 100 || string(delivery.Result) != "preserve" {
		t.Fatal("dependency inspection altered claimed receipt", err)
	}
}

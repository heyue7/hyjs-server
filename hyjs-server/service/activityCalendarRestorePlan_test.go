package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"hyjs-server/db"
	"hyjs-server/model"
)

func formal110PlanFixture() (db.AdminGameServer, []db.AdminActivityOccurrence, time.Time) {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	old := time.Date(2026, 10, 11, 0, 0, 0, 0, zone)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, zone)
	form := adminJSON(model.AdminActivityForm{Alias: "成长投放V12", Source: "成长投放V12 / 2026-10-26", TemplateID: "recharge-total-original", Name: "累计充值", Mode: "recharge-total"})
	key := "character_plan_ee96fab9913045899f9c5e29"
	rows := []db.AdminActivityOccurrence{{ID: "personal", ServerID: 1, TemplateID: formal110PersonalTemplate, Timing: "character-day", StartDay: 1, DurationDays: 14, Mode: "recharge-total", ConsumerKey: key, ConsumerID: "character-plan:" + key, Enabled: true, FormJSON: form}}
	starts := []time.Time{
		time.Date(2026, 10, 31, 0, 0, 0, 0, zone),
		time.Date(2026, 12, 1, 0, 0, 0, 0, zone),
		time.Date(2026, 12, 31, 0, 0, 0, 0, zone),
		time.Date(2027, 1, 31, 0, 0, 0, 0, zone),
		time.Date(2027, 3, 3, 0, 0, 0, 0, zone),
		time.Date(2027, 3, 31, 0, 0, 0, 0, zone),
		time.Date(2027, 5, 1, 0, 0, 0, 0, zone),
	}
	// Reverse source row order to prove association uses source time, not ID order.
	for i := len(starts) - 1; i >= 0; i-- {
		key := fmt.Sprintf("rechargeTotal_p4_admin_schedule_%d", i)
		end := starts[i].AddDate(0, 0, 3).UnixMilli()
		rows = append(rows, db.AdminActivityOccurrence{ID: fmt.Sprintf("monthly-%d", i), ServerID: 1, TemplateID: formal110MonthlyTemplate, Timing: "calendar", DurationDays: 3, Mode: "recharge-total", ConsumerKey: key, ConsumerID: "period:" + key, StartAt: starts[i].UnixMilli(), EndAt: end, ClaimEndAt: end, Enabled: true, FormJSON: form})
	}
	for i, day := range []int64{1, 15} {
		rows = append(rows, db.AdminActivityOccurrence{ID: fmt.Sprintf("wheel-%d", i), ServerID: 1, Timing: "character-day", StartDay: day, DurationDays: 14, Mode: "turntable", FormJSON: adminJSON(model.AdminActivityForm{Name: "幸运大转盘"}), Enabled: true})
	}
	rows = append(rows,
		db.AdminActivityOccurrence{ID: "opening", ServerID: 1, Timing: "character-day", StartDay: 1, DurationDays: 14, Mode: "opening-14", FormJSON: adminJSON(model.AdminActivityForm{Name: "开服"}), Enabled: true},
		db.AdminActivityOccurrence{ID: "other-recharge", ServerID: 1, TemplateID: "other-template", Timing: "character-day", StartDay: 5, DurationDays: 9, Mode: "recharge-total", FormJSON: adminJSON(model.AdminActivityForm{Name: "累计充值"}), Enabled: true},
	)
	return db.AdminGameServer{ID: 1, OpenAt: old.UnixMilli()}, rows, now
}

func TestFormal110RestoreApprovedWindows(t *testing.T) {
	realm, rows, now := formal110PlanFixture()
	plan := activityCalendarRestorePlanWithFormal110(realm, db.ServerGameClock{}, db.AdminActivityScheduleRevision{}, rows, nil, nil, now, true)
	if !plan.Formal110 || len(plan.Blockers) != 0 || len(plan.Windows) != len(rows) {
		t.Fatalf("unexpected formal plan: %+v", plan)
	}
	defaultPlan := activityCalendarRestorePlan(realm, db.ServerGameClock{}, db.AdminActivityScheduleRevision{}, rows, nil, nil, now)
	if defaultPlan.Formal110 || defaultPlan.Digest == plan.Digest {
		t.Fatal("formal mode was not isolated from the ordinary candidate")
	}
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	for i, window := range plan.Windows {
		switch {
		case window.OccurrenceID == "personal":
			start := time.Date(2026, 11, 1, 0, 0, 0, 0, zone)
			end := start.AddDate(0, 0, 3).UnixMilli()
			if window.StartDay != 1 || window.DurationDays != 3 || window.After != [3]int64{start.UnixMilli(), end, end} || defaultPlan.Windows[i].DurationDays != 14 {
				t.Fatalf("wrong personal window: %+v", window)
			}
		case strings.HasPrefix(window.OccurrenceID, "monthly-"):
			var n int
			if _, err := fmt.Sscanf(window.OccurrenceID, "monthly-%d", &n); err != nil {
				t.Fatal(err)
			}
			start := time.Date(2026, time.November+time.Month(n), 15, 0, 0, 0, 0, zone)
			end := start.AddDate(0, 0, 3).UnixMilli()
			if window.After != [3]int64{start.UnixMilli(), end, end} {
				t.Fatalf("wrong month %d window: %+v", n, window)
			}
		default:
			if !reflect.DeepEqual(window, defaultPlan.Windows[i]) {
				t.Fatalf("unrelated schedule changed: %+v vs %+v", window, defaultPlan.Windows[i])
			}
		}
	}
}

func TestFormal110RestoreRejectsWrongPublication(t *testing.T) {
	tests := []struct {
		name   string
		change func(*db.AdminGameServer, *[]db.AdminActivityOccurrence, *time.Time)
		want   string
	}{
		{"missing-month", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) {
			*rows = append((*rows)[:1], (*rows)[2:]...)
		}, "expected one personal and seven monthly"},
		{"wrong-source", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) {
			(*rows)[0].FormJSON = adminJSON(model.AdminActivityForm{Alias: "wrong", TemplateID: "recharge-total-original"})
		}, "unexpected publication identity"},
		{"wrong-art-template", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) {
			(*rows)[0].FormJSON = adminJSON(model.AdminActivityForm{Alias: "成长投放V12", Source: "成长投放V12 / 2026-10-26", TemplateID: formal110PersonalTemplate})
		}, "unexpected publication identity"},
		{"wrong-personal-consumer", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) {
			(*rows)[0].ConsumerID = "period:wrong"
		}, "unexpected personal consumer"},
		{"wrong-month-timing", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) {
			(*rows)[1].Timing = "character-day"
		}, "unexpected monthly consumer"},
		{"wrong-month-window", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) { (*rows)[1].EndAt++ }, "unexpected monthly consumer"},
		{"duplicate-month-start", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) {
			(*rows)[1].StartAt = (*rows)[2].StartAt
		}, "duplicate monthly"},
		{"restaurant-enabled", func(_ *db.AdminGameServer, rows *[]db.AdminActivityOccurrence, _ *time.Time) {
			*rows = append(*rows, db.AdminActivityOccurrence{ID: "restaurant", Mode: "stamina-meal", FormJSON: []byte(`{}`)})
		}, "disable restaurant"},
		{"wrong-server", func(realm *db.AdminGameServer, _ *[]db.AdminActivityOccurrence, _ *time.Time) { realm.ID = 2 }, "requires server 1"},
		{"wrong-date", func(_ *db.AdminGameServer, _ *[]db.AdminActivityOccurrence, now *time.Time) {
			*now = now.AddDate(0, 0, 1)
		}, "requires server 1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			realm, rows, now := formal110PlanFixture()
			tc.change(&realm, &rows, &now)
			plan := activityCalendarRestorePlanWithFormal110(realm, db.ServerGameClock{}, db.AdminActivityScheduleRevision{}, rows, nil, nil, now, true)
			if !strings.Contains(strings.Join(plan.Blockers, "\n"), tc.want) {
				t.Fatalf("missing blocker %q: %+v", tc.want, plan.Blockers)
			}
		})
	}
}

func TestFormal110RestoreDigestBindsModeEvenWithIdenticalWindows(t *testing.T) {
	realm, rows, now := formal110PlanFixture()
	realm.OpenAt = time.Date(2026, 11, 1, 0, 0, 0, 0, now.Location()).UnixMilli()
	rows[0].DurationDays = 3
	for i := 1; i <= 7; i++ {
		start := time.Date(2026, time.November+time.Month(7-i), 15, 0, 0, 0, 0, now.Location())
		rows[i].StartAt = start.UnixMilli()
		rows[i].EndAt = start.AddDate(0, 0, 3).UnixMilli()
		rows[i].ClaimEndAt = rows[i].EndAt
	}
	formal := activityCalendarRestorePlanWithFormal110(realm, db.ServerGameClock{}, db.AdminActivityScheduleRevision{}, rows, nil, nil, now, true)
	ordinary := activityCalendarRestorePlan(realm, db.ServerGameClock{}, db.AdminActivityScheduleRevision{}, rows, nil, nil, now)
	if len(formal.Blockers) != 0 || !reflect.DeepEqual(formal.Windows, ordinary.Windows) || formal.Digest == ordinary.Digest {
		t.Fatalf("identical windows must have mode-bound digests: formal=%+v ordinary=%+v", formal, ordinary)
	}
}

func TestFormal110RestoreRequiresEmptyRoleState(t *testing.T) {
	if !(formal110EmptySnapshot{}).empty() {
		t.Fatal("empty role state rejected")
	}
	for _, snapshot := range []formal110EmptySnapshot{{Profiles: 1}, {RoleLinks: 1}, {Issued: 1}, {Anchors: 1}, {CommerceStates: 1}, {CommerceStocks: 1}, {CommerceReceipts: 1}, {ActivityOperations: 1}, {ActivityBindings: 1}, {ActivityCycles: 1}, {RushStates: 1}} {
		if snapshot.empty() {
			t.Fatalf("role state was accepted: %+v", snapshot)
		}
	}
}

func TestFormal110RestoreOpeningDateUsesBeijingMidnight(t *testing.T) {
	before := time.Date(2026, 10, 31, 15, 59, 59, 999000000, time.UTC)
	opening := time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC)
	closing := time.Date(2026, 11, 1, 16, 0, 0, 0, time.UTC)
	if formal110OpeningDate(1, before) || !formal110OpeningDate(1, opening) || formal110OpeningDate(1, closing) || formal110OpeningDate(2, opening) {
		t.Fatal("formal migration date or server guard is incorrect")
	}
}

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

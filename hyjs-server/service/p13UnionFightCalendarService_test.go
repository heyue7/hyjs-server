package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func p13CalendarAt(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04", value, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		t.Fatal(err)
	}
	return at
}
func p13CalendarFixture(t *testing.T) (*p1Fixture, *db.P13UnionFightSchedule) {
	t.Helper()
	f, _ := p13GuildSetup(t)
	if err := f.s.InitializeP13UnionFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightSchedule{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	row := &db.P13UnionFightSchedule{Day: "2026-11-01", RegistrationDay: "2026-10-30", Enabled: true, Version: "explicit-sunday-v1", PublishedAt: 1, EnrollmentStartAt: p13CalendarAt(t, "2026-10-30 12:00").UnixMilli(), EnrollmentEndAt: p13CalendarAt(t, "2026-11-01 19:50").UnixMilli(), FirstStartAt: p13CalendarAt(t, "2026-11-01 20:00").UnixMilli(), SecondStartAt: p13CalendarAt(t, "2026-11-01 21:00").UnixMilli(), DurationMillis: 1800000, PreparationMillis: 300000, Source: "explicit custom Sunday lifecycle fixture"}
	t.Cleanup(func() {
		db.GetIns().Where("day=?", row.Day).Delete(&db.P13UnionFightSchedule{})
		db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false)
	})
	return f, row
}

func TestP13UFCalendarLegacyParityAndCustomBeijingBoundaries(t *testing.T) {
	_, row := p13CalendarFixture(t)
	for _, value := range []string{"2026-10-26 00:00", "2026-10-27 00:29", "2026-10-27 00:30", "2026-10-28 13:00", "2026-10-31 23:59", "2026-11-01 12:00"} {
		at := p13CalendarAt(t, value)
		got, err := p13UFCalendar(db.GetIns(), at)
		start, day, stage, end := p13UFClock(at)
		if err != nil || got.Custom || !got.DayStart.Equal(start) || got.MatchDay != day || got.RegistrationDay != p13UFDay(at) || got.Stage != stage || got.Deadline != end {
			t.Fatal("legacy calendar changed", value, got, err)
		}
	}
	if err := db.GetIns().Create(row).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at       string
		stage    int32
		deadline int64
	}{
		{"2026-10-30 11:59", 0, row.EnrollmentStartAt},
		{"2026-10-30 12:00", 1, row.EnrollmentEndAt},
		{"2026-11-01 19:49", 1, row.EnrollmentEndAt},
		{"2026-11-01 19:50", 2, row.FirstStartAt - row.PreparationMillis},
		{"2026-11-01 19:55", 0, row.FirstStartAt},
		{"2026-11-01 20:00", 0, row.FirstStartAt + row.DurationMillis},
		{"2026-11-01 20:30", 0, row.SecondStartAt},
	} {
		at := p13CalendarAt(t, tc.at).In(time.FixedZone("other", -7*3600))
		got, err := p13UFCalendar(db.GetIns(), at)
		if err != nil || got.Stage != tc.stage || got.Deadline != tc.deadline || got.RegistrationDay != row.RegistrationDay || got.MatchDay != row.Day {
			t.Fatal("custom boundary/timezone mismatch", tc.at, got, err)
		}
	}
	// Current global preparation edits cannot rewrite published stages.
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("preparation_millis", 900000).Error; err != nil {
		t.Fatal(err)
	}
	got, err := p13UFCalendar(db.GetIns(), p13CalendarAt(t, "2026-11-01 19:50"))
	if err != nil || got.Deadline != row.FirstStartAt-300000 {
		t.Fatal("frozen preparation drifted", err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	registration, err := p13UFRegistrationDay(db.GetIns(), row.Day)
	if err != nil || registration != row.RegistrationDay {
		t.Fatal("old match lost explicit registration identity", registration, err)
	}
}

func TestP13UFCalendarCustomValidationAndHumanTimeUnits(t *testing.T) {
	now := p13CalendarAt(t, "2026-10-26 12:00")
	req := model.AdminGuildScheduleSaveRequest{AdminGuildScheduleForm: model.AdminGuildScheduleForm{Day: "2026-11-01", EnrollmentStart: "2026-10-30 12:00", EnrollmentEnd: "2026-11-01 19:55", FirstStartTime: "20:00", SecondStartTime: "21:00", Enabled: true, Version: "future-v1"}, Reason: "explicit future schedule"}
	row, err := adminGuildScheduleValue(req, now, 300000)
	if err != nil || row.DurationMillis != 1800000 || row.RegistrationDay != "2026-10-30" || row.PreparationMillis != 300000 {
		t.Fatal("default30 minutes / frozen preparation", row, err)
	}
	cases := map[string]func(*model.AdminGuildScheduleSaveRequest){
		"overlap":           func(r *model.AdminGuildScheduleSaveRequest) { r.SecondStartTime = "20:20" },
		"late-cutoff":       func(r *model.AdminGuildScheduleSaveRequest) { r.EnrollmentEnd = "2026-11-01 19:56" },
		"cross-midnight":    func(r *model.AdminGuildScheduleSaveRequest) { r.SecondStartTime = "23:45" },
		"invalid-date":      func(r *model.AdminGuildScheduleSaveRequest) { r.Day = "2026-02-30" },
		"duration31":        func(r *model.AdminGuildScheduleSaveRequest) { r.DurationMinutes = 31 },
		"negative-duration": func(r *model.AdminGuildScheduleSaveRequest) { r.DurationMinutes = -1 },
		"window-reversed":   func(r *model.AdminGuildScheduleSaveRequest) { r.EnrollmentStart = "2026-11-02 12:00" },
		"past-start":        func(r *model.AdminGuildScheduleSaveRequest) { r.EnrollmentStart = "2026-10-26 11:00" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := req
			mutate(&changed)
			if _, err := adminGuildScheduleValue(changed, now, 300000); err == nil {
				t.Fatal("invalid schedule accepted")
			}
		})
	}
}

func TestP13UFCalendarSundayRealDeclareSignupFreeze(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, true)
	p13UFOpen(t)
	now := p13CalendarAt(t, "2026-11-01 12:00")
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("日历真实公会")}, 1, now)
	guildID := p13GuildID(t, f, ids[0], now)
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guildID).Update("level", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightSchedule{}).Error; err != nil {
		t.Fatal(err)
	}
	row := db.P13UnionFightSchedule{Day: "2026-11-02", RegistrationDay: "2026-11-01", Enabled: true, Version: "sunday-real-declare", PublishedAt: 1, EnrollmentStartAt: now.UnixMilli(), EnrollmentEndAt: now.Add(time.Hour).UnixMilli(), FirstStartAt: p13CalendarAt(t, "2026-11-02 20:00").UnixMilli(), SecondStartAt: p13CalendarAt(t, "2026-11-02 21:00").UnixMilli(), DurationMillis: 1800000, PreparationMillis: 300000, Source: "explicit Sunday signup to Monday battle"}
	if err := db.GetIns().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.GetIns().Where("day=?", row.Day).Delete(&db.P13UnionFightSchedule{})
		db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false)
	})
	p13UFCall(t, f, ids[0], &pb.CSUFUnionApply{}, 2, now)
	p13UFCall(t, f, ids[0], &pb.CSUFPlayerApply{}, 3, now.Add(time.Second))
	if err := f.s.TickP13UnionFight(f.ctx, time.UnixMilli(row.EnrollmentEndAt)); err != nil {
		t.Fatal(err)
	}
	var entry db.P13UnionFightEntry
	if err := db.GetIns().First(&entry, "guild_id=? AND day=?", guildID, row.RegistrationDay).Error; err != nil || entry.Deadline != row.EnrollmentEndAt || entry.FrozenAt == 0 || entry.EligibleCount != 1 || p6Digest(entry.FrozenTables) != entry.ConfigurationHash {
		t.Fatal("real custom roster did not freeze at cutoff", entry.EligibleCount, err)
	}
	if _, err := f.s.p13UFMutate(f.ctx, ids[0], &pb.CSUFPlayerApply{}, 4, time.UnixMilli(row.EnrollmentEndAt)); err == nil {
		t.Fatal("signup accepted after custom cutoff")
	}
}

func TestP13UFCalendarAutoApplyBoundedEmptyRosterBatch(t *testing.T) {
	f, row := p13CalendarFixture(t)
	p13GuildOpen(t, true)
	p13UFOpen(t)
	if err := db.GetIns().Create(row).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	// Explicit guild ledger fixtures isolate batching; no fictitious member,
	// battle, winner, resource or reward is produced by these empty rosters.
	for i := 0; i < 17; i++ {
		id := fmt.Sprintf("calendar-auto-%02d", i)
		ids = append(ids, id)
		yes := true
		if err := db.GetIns().Create(&db.P13Guild{ID: id, Name: id, Level: 2, UnionFightAutoApply: &yes}).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.GetIns().Where("guild_id IN ?", ids).Delete(&db.P13UnionFightEntry{})
		db.GetIns().Where("id IN ?", ids).Delete(&db.P13Guild{})
	})
	at := time.UnixMilli(row.EnrollmentStartAt + 600000)
	for _, expected := range []int64{8, 16, 17, 17} {
		if err := f.s.TickP13UnionFight(f.ctx, at); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.GetIns().Model(&db.P13UnionFightEntry{}).Where("day=? AND guild_id IN ?", row.RegistrationDay, ids).Count(&count).Error; err != nil || count != expected {
			t.Fatal("bounded automatic batch / already declared exclusion", expected, count, err)
		}
	}
}

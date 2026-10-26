package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

// This fixture performs real declaration, player enrollment, cutoff freezing and
// matching. It creates no terminal, winner, prestige or award.
func TestP13UFCalendarCoverageRealCrossWeekAndMonthGrouping(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, true)
	p13UFOpen(t)
	for _, table := range []any{&db.P13UnionFightSchedule{}, &db.P13UnionFightEntry{}, &db.P13UnionFightFighter{}, &db.P13UnionFightGroup{}, &db.P13UnionFightMatch{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Updates(map[string]any{"custom_calendar_enabled": true, "match_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	at := p13CalendarAt(t, "2026-10-30 12:00")
	schedule := db.P13UnionFightSchedule{Day: "2026-11-02", RegistrationDay: "2026-10-30", Enabled: true, Version: "coverage-real-v1", PublishedAt: 1, EnrollmentStartAt: at.UnixMilli(), EnrollmentEndAt: p13CalendarAt(t, "2026-11-01 19:00").UnixMilli(), FirstStartAt: p13CalendarAt(t, "2026-11-02 20:00").UnixMilli(), SecondStartAt: p13CalendarAt(t, "2026-11-02 21:00").UnixMilli(), DurationMillis: 1800000, PreparationMillis: 300000, Source: "explicit cross-week/cross-month real registration fixture"}
	if err := db.GetIns().Create(&schedule).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false)
	})
	guilds := map[string]bool{}
	for i, id := range ids {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("跨周覆盖%d", i))}, 1, at)
		guild := p13GuildID(t, f, id, at)
		guilds[guild] = true
		if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; err != nil {
			t.Fatal(err)
		}
		p13UFCall(t, f, id, &pb.CSUFUnionApply{}, 2, at)
		p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, 3, at.Add(time.Second))
	}
	if err := f.s.TickP13UnionFight(f.ctx, time.UnixMilli(schedule.EnrollmentEndAt)); err != nil {
		t.Fatal(err)
	}
	var matches []db.P13UnionFightMatch
	if err := db.GetIns().Find(&matches).Error; err != nil || len(matches) == 0 {
		t.Fatal("real matching missing", len(matches), err)
	}
	var groups []db.P13UnionFightGroup
	if err := db.GetIns().Find(&groups).Error; err != nil || len(groups) != len(ids) {
		t.Fatal("real group assignments", len(groups), err)
	}
	for _, group := range groups {
		if !guilds[group.GuildID] || group.WeekStart != "2026-11-02" {
			t.Fatal("group used registration week", group)
		}
	}
	for _, match := range matches {
		if match.Day != schedule.Day {
			t.Fatal("match day drift", match.Day)
		}
	}
	read := func(from, to string) []p13UFCalendarCoverageRow {
		t.Helper()
		rows, err := p13UFCalendarCoverage(db.GetIns(), p13CalendarAt(t, from+" 00:00"), p13CalendarAt(t, to+" 00:00"), matches)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if rows := read("2026-10-26", "2026-11-02"); len(rows) != 0 {
		t.Fatal("future custom signup became legacy in old week", len(rows))
	}
	if rows := read("2026-10-01", "2026-11-01"); len(rows) != 0 {
		t.Fatal("future custom signup became old-month participation", len(rows))
	}
	rows := read("2026-11-02", "2026-11-09")
	if len(rows) != len(ids) {
		t.Fatal("cross-week entry omitted", len(rows))
	}
	for _, row := range rows {
		if row.MatchDay != schedule.Day || row.Entry.Day != schedule.RegistrationDay || row.Entry.FrozenAt == 0 || row.Entry.EligibleCount != 1 {
			t.Fatal("not real frozen entry", row)
		}
	}
	if rows := read("2026-11-01", "2026-12-01"); len(rows) != len(ids) {
		t.Fatal("cross-month entry omitted", len(rows))
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if rows := read("2026-11-02", "2026-11-09"); len(rows) != len(ids) {
		t.Fatal("global switch rewrote historical ownership")
	}
	// A finite identity-only eligible registration has no assigned group yet.
	// It must stop weekly certification before any round terminal is consumed.
	rawSchedule, _ := json.Marshal(schedule)
	unassigned := db.P13UnionFightEntry{GuildID: "coverage-unassigned-identity", Day: schedule.RegistrationDay, FrozenAt: 1, EligibleCount: 1, Source: "后台明确发布自定义日历；冻结报名时排期：" + string(rawSchedule)}
	if err := db.GetIns().Create(&unassigned).Error; err != nil {
		t.Fatal(err)
	}
	policy := db.P13UnionFightWeeklyPolicy{WeekStart: "2026-11-02", Group: matches[0].Group, Enabled: true, Version: "coverage-only-reward-publication", Source: "explicit reward fixture without outcome", PublishedAt: 1, Rewards: `[{"rank":1,"rewards":"item_089,1"}]`}
	rules, _ := json.Marshal(policy)
	offer := db.P13UnionFightWeeklyOffer{WeekStart: policy.WeekStart, Group: policy.Group, Rules: rules, RulesHash: p6Digest(rules), FrozenAt: 2, FirstMatchID: matches[0].ID}
	database := db.GetIns()
	originalLogger := database.Config.Logger
	recorder := &p13LifecycleSQLLog{Interface: originalLogger}
	database.Config.Logger = recorder
	result, settleErr := f.s.p13UFWeeklySettle(database, &offer, p13CalendarAt(t, "2026-11-08 12:00"))
	database.Config.Logger = originalLogger
	if settleErr != nil || result != nil {
		t.Fatal("unassigned eligible declaration certified", result, settleErr)
	}
	for _, sql := range recorder.SQL {
		if strings.Contains(sql, "p13_union_fight_settlement_results") {
			t.Fatal("unassigned coverage did not stop before terminal consumption", sql)
		}
	}
	if err := db.GetIns().Where("day=?", schedule.Day).Delete(&db.P13UnionFightSchedule{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p13UFCalendarCoverage(db.GetIns(), p13CalendarAt(t, "2026-11-02 00:00"), p13CalendarAt(t, "2026-11-09 00:00"), matches); err == nil {
		t.Fatal("lost explicit publication silently treated as legacy")
	}
}

// These finite entries prove calendar identity/paging only. No roster, result or
// terminal authority is fabricated; unprocessed eligibility remains observable.
func TestP13UFCalendarCoverageLegacyMetadataPagingAndConflict(t *testing.T) {
	_, _ = p13CalendarFixture(t)
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightEntry{}).Error; err != nil {
		t.Fatal(err)
	}
	entries := make([]db.P13UnionFightEntry, 130)
	for i := range entries {
		entries[i] = db.P13UnionFightEntry{GuildID: fmt.Sprintf("coverage-identity-%03d", i), Day: "2026-10-26", FrozenAt: 1, EligibleCount: 1, MatchProcessed: false, Source: "identity-only legacy coverage fixture"}
	}
	if err := db.GetIns().CreateInBatches(entries, 50).Error; err != nil {
		t.Fatal(err)
	}
	from, to := p13CalendarAt(t, "2026-10-26 00:00"), p13CalendarAt(t, "2026-11-02 00:00")
	rows, err := p13UFCalendarCoverage(db.GetIns(), from, to, nil)
	if err != nil || len(rows) != 130 {
		t.Fatal("bounded metadata page lost entries", len(rows), err)
	}
	for _, row := range rows {
		if row.MatchDay != "2026-10-27" || row.Entry.MatchProcessed {
			t.Fatal("legacy +1/unprocessed identity changed", row)
		}
	}
	// The custom source snapshot alone never authorizes a missing publication.
	snapshot := db.P13UnionFightSchedule{Day: "2026-10-29", RegistrationDay: "2026-10-26"}
	raw, _ := json.Marshal(snapshot)
	if err := db.GetIns().Model(&db.P13UnionFightEntry{}).Where("guild_id=?", entries[0].GuildID).Update("source", "后台明确发布自定义日历；冻结报名时排期："+string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p13UFCalendarCoverage(db.GetIns(), from, to, nil); err == nil || !strings.Contains(err.Error(), "排期归属") {
		t.Fatal("missing custom publication accepted", err)
	}
}

// Identity-only publications exercise same-day windows and ambiguous mappings;
// no scene entry, battle result, prestige or reward is created.
func TestP13UFCalendarCoverageSameDayAndConflictingPublication(t *testing.T) {
	_, _ = p13CalendarFixture(t)
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightEntry{}).Error; err != nil {
		t.Fatal(err)
	}
	row := db.P13UnionFightSchedule{Day: "2026-10-28", RegistrationDay: "2026-10-28", Enabled: false, Version: "same-day-coverage-only", PublishedAt: 1, EnrollmentStartAt: p13CalendarAt(t, "2026-10-28 12:00").UnixMilli(), EnrollmentEndAt: p13CalendarAt(t, "2026-10-28 18:00").UnixMilli(), FirstStartAt: p13CalendarAt(t, "2026-10-28 20:00").UnixMilli(), SecondStartAt: p13CalendarAt(t, "2026-10-28 21:00").UnixMilli(), DurationMillis: 1800000, PreparationMillis: 300000, Source: "retained historical identity fixture"}
	if err := db.GetIns().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(row)
	entry := db.P13UnionFightEntry{GuildID: "same-day-identity", Day: row.RegistrationDay, FrozenAt: 1, EligibleCount: 1, Source: "后台明确发布自定义日历；冻结报名时排期：" + string(raw)}
	if err := db.GetIns().Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	from, to := p13CalendarAt(t, "2026-10-26 00:00"), p13CalendarAt(t, "2026-11-02 00:00")
	rows, err := p13UFCalendarCoverage(db.GetIns(), from, to, nil)
	if err != nil || len(rows) != 1 || rows[0].MatchDay != row.Day {
		t.Fatal("retained same-day/disabled mapping", rows, err)
	}
	duplicate := row
	duplicate.Day = "2026-10-29"
	duplicate.FirstStartAt = p13CalendarAt(t, "2026-10-29 20:00").UnixMilli()
	duplicate.SecondStartAt = p13CalendarAt(t, "2026-10-29 21:00").UnixMilli()
	if err := db.GetIns().Create(&duplicate).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p13UFCalendarCoverage(db.GetIns(), from, to, nil); err == nil {
		t.Fatal("conflicting historical registration mapping accepted")
	}
}

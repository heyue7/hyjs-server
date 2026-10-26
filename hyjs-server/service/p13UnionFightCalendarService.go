package service

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"sort"
	"strings"
	"time"
)

// p13UFCalendarWindow resolves the single enrollment/roster/match calendar.
// Legacy Clock/Day remain pure helpers and keep their existing weekday contract.
type p13UFCalendarWindow struct {
	Custom          bool
	DayStart        time.Time
	RegistrationDay string
	MatchDay        string
	Stage           int32
	Deadline        int64
	Schedule        *db.P13UnionFightSchedule
}

func p13UFCustomScheduleValid(row *db.P13UnionFightSchedule, preparation int64) error {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	day, err := time.ParseInLocation("2006-01-02", row.Day, zone)
	registration, e2 := time.ParseInLocation("2006-01-02", row.RegistrationDay, zone)
	if err != nil || e2 != nil || day.Format("2006-01-02") != row.Day || registration.Format("2006-01-02") != row.RegistrationDay || row.Version == "" || row.Source == "" || row.PublishedAt <= 0 || preparation <= 0 || preparation > 86400000 || row.DurationMillis < 60000 || row.DurationMillis > 1800000 || row.EnrollmentStartAt <= 0 || row.EnrollmentStartAt >= row.EnrollmentEndAt || time.UnixMilli(row.EnrollmentStartAt).In(zone).Format("2006-01-02") != row.RegistrationDay || row.FirstStartAt < day.UnixMilli() || row.FirstStartAt >= day.AddDate(0, 0, 1).UnixMilli() || row.EnrollmentEndAt > row.FirstStartAt-preparation || row.SecondStartAt < row.FirstStartAt+row.DurationMillis || row.SecondStartAt >= day.AddDate(0, 0, 1).UnixMilli() || row.SecondStartAt+row.DurationMillis > day.AddDate(0, 0, 1).UnixMilli() {
		return errors.New("公会战排期日期、报名窗口或两场时间无效")
	}
	return nil
}

func p13UFCalendar(tx *gorm.DB, now time.Time) (*p13UFCalendarWindow, error) {
	start, day, stage, deadline := p13UFClock(now)
	out := &p13UFCalendarWindow{DayStart: start, RegistrationDay: p13UFDay(now), MatchDay: day, Stage: stage, Deadline: deadline}
	var policy db.P13UnionFightPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	if !policy.CustomCalendarEnabled {
		return out, nil
	}
	out.Custom = true
	out.RegistrationDay = ""
	out.MatchDay = ""
	out.Stage = 0
	out.Deadline = 0
	var rows []db.P13UnionFightSchedule
	// Prefer a started enrollment through the end of its fight day. Future
	// publications cannot replace an active/frozen roster with a different day.
	err := tx.Where("enabled=? AND day>=? AND enrollment_start_at<=? AND second_start_at+duration_millis>?", true, start.Format("2006-01-02"), now.UnixMilli(), now.UnixMilli()).Order("enrollment_start_at").Limit(2).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) > 1 {
		return nil, errors.New("公会战排期窗口冲突")
	}
	if len(rows) == 0 {
		// Keep the completed fight day visible as original stage7 until another
		// enrollment actually starts; a future draft cannot hide that final phase.
		if err = tx.Where("enabled=? AND day=? AND enrollment_start_at<=?", true, start.Format("2006-01-02"), now.UnixMilli()).Order("second_start_at DESC").Limit(1).Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	if len(rows) == 0 {
		if err = tx.Where("enabled=? AND enrollment_start_at>?", true, now.UnixMilli()).Order("enrollment_start_at").Limit(1).Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	if len(rows) == 0 {
		return out, nil
	}
	row := rows[0]
	if err = p13UFCustomScheduleValid(&row, row.PreparationMillis); err != nil {
		return nil, err
	}
	out.Schedule = &row
	out.RegistrationDay = row.RegistrationDay
	out.MatchDay = row.Day
	out.DayStart, _ = time.ParseInLocation("2006-01-02", row.Day, start.Location())
	switch {
	case now.UnixMilli() < row.EnrollmentStartAt:
		out.Deadline = row.EnrollmentStartAt
	case now.UnixMilli() < row.EnrollmentEndAt:
		out.Stage = constUtil.P13UnionFightStageDeclare
		out.Deadline = row.EnrollmentEndAt
	case now.UnixMilli() < row.FirstStartAt-row.PreparationMillis:
		out.Stage = constUtil.P13UnionFightStageMatch
		out.Deadline = row.FirstStartAt - row.PreparationMillis
	case now.UnixMilli() < row.FirstStartAt:
		out.Deadline = row.FirstStartAt
	case now.UnixMilli() < row.FirstStartAt+row.DurationMillis:
		out.Deadline = row.FirstStartAt + row.DurationMillis
	case now.UnixMilli() < row.SecondStartAt:
		out.Deadline = row.SecondStartAt
	default:
		out.Deadline = row.SecondStartAt + row.DurationMillis
	}
	return out, nil
}

// p13UFRegistrationDay always honors retained explicit schedule identity even
// after the global custom switch changes. Frozen matches never depend on it.
func p13UFRegistrationDay(tx *gorm.DB, matchDay string) (string, error) {
	var row db.P13UnionFightSchedule
	err := tx.First(&row, "day=?", matchDay).Error
	if err == nil && row.RegistrationDay != "" {
		return row.RegistrationDay, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	day, err := time.ParseInLocation("2006-01-02", matchDay, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		return "", err
	}
	return day.AddDate(0, 0, -1).Format("2006-01-02"), nil
}

// p13UFCalendarEntry is period coverage metadata, not a battle or outcome.
type p13UFCalendarEntry struct {
	GuildID        string
	Day            string
	FrozenAt       int64
	EligibleCount  int64
	MatchProcessed bool
	Source         string
}
type p13UFCalendarCoverageRow struct {
	Entry    p13UFCalendarEntry
	MatchDay string
}

// p13UFCalendarCoverage uses retained publication identity, independent of the
// current calendar switch. Queries are date/key bounded and page only metadata.
func p13UFCalendarCoverage(tx *gorm.DB, start, end time.Time, matches []db.P13UnionFightMatch) ([]p13UFCalendarCoverageRow, error) {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	start, end = start.In(zone), end.In(zone)
	if !end.After(start) || end.Sub(start) > 31*24*time.Hour {
		return nil, errors.New("公会战历史日历覆盖区间无效")
	}
	fail := func() error { return errors.New("公会战历史报名与冻结排期归属损坏") }
	keys := map[string]bool{}
	for at := start; at.Before(end); at = at.AddDate(0, 0, 1) {
		keys[at.AddDate(0, 0, -1).Format("2006-01-02")] = true
	}
	var inPeriod []db.P13UnionFightSchedule
	if err := tx.Where("day>=? AND day<?", start.Format("2006-01-02"), end.Format("2006-01-02")).Order("day").Limit(32).Find(&inPeriod).Error; err != nil {
		return nil, err
	}
	if len(inPeriod) > 31 {
		return nil, fail()
	}
	for _, row := range inPeriod {
		if row.RegistrationDay != "" {
			keys[row.RegistrationDay] = true
		}
	}
	// Frozen match rosters retain signup days even when a custom publication was
	// lost. Read only JSON identity fields, never decode or regenerate Battle PB.
	for _, match := range matches {
		if match.Day < start.Format("2006-01-02") || match.Day >= end.Format("2006-01-02") {
			continue
		}
		var roster struct{ Left, Right []struct{ Day string } }
		if err := json.Unmarshal(match.Roster, &roster); err != nil {
			return nil, fail()
		}
		for _, side := range [][]struct{ Day string }{roster.Left, roster.Right} {
			for _, fighter := range side {
				if fighter.Day != "" {
					keys[fighter.Day] = true
				}
			}
		}
	}
	dates := make([]string, 0, len(keys))
	for day := range keys {
		dates = append(dates, day)
	}
	sort.Strings(dates)
	if len(dates) > 128 {
		return nil, fail()
	}
	// Include explicit publications outside the period whose registration day is
	// a legacy candidate. They must not masquerade as a next-day legacy entry.
	var explicit []db.P13UnionFightSchedule
	if err := tx.Where("registration_day IN ?", dates).Order("day").Limit(129).Find(&explicit).Error; err != nil {
		return nil, err
	}
	if len(explicit) > 128 {
		return nil, fail()
	}
	byRegistration := map[string]db.P13UnionFightSchedule{}
	byDay := map[string]db.P13UnionFightSchedule{}
	for _, row := range append(inPeriod, explicit...) {
		byDay[row.Day] = row
		if row.RegistrationDay == "" {
			continue
		}
		if err := p13UFCustomScheduleValid(&row, row.PreparationMillis); err != nil {
			return nil, fail()
		}
		if previous, ok := byRegistration[row.RegistrationDay]; ok && previous != row {
			return nil, fail()
		}
		byRegistration[row.RegistrationDay] = row
	}
	result := []p13UFCalendarCoverageRow{}
	lastDay, lastGuild := "", ""
	for {
		if tx.Statement.Context.Err() != nil {
			return nil, tx.Statement.Context.Err()
		}
		var entries []p13UFCalendarEntry
		query := tx.Model(&db.P13UnionFightEntry{}).Select("guild_id,day,frozen_at,eligible_count,match_processed,source").Where("day IN ?", dates)
		if lastDay != "" {
			query = query.Where("day>? OR (day=? AND guild_id>?)", lastDay, lastDay, lastGuild)
		}
		if err := query.Order("day,guild_id").Limit(128).Find(&entries).Error; err != nil {
			return nil, err
		}
		for _, entry := range entries {
			matchDay := ""
			publication, custom := byRegistration[entry.Day]
			const prefix = "后台明确发布自定义日历；冻结报名时排期："
			if strings.HasPrefix(entry.Source, prefix) {
				var frozen db.P13UnionFightSchedule
				if json.Unmarshal([]byte(strings.TrimPrefix(entry.Source, prefix)), &frozen) != nil || !custom || frozen != publication || frozen.RegistrationDay != entry.Day {
					return nil, fail()
				}
			}
			if custom {
				matchDay = publication.Day
			} else {
				at, err := time.ParseInLocation("2006-01-02", entry.Day, zone)
				if err != nil {
					return nil, fail()
				}
				matchDay = at.AddDate(0, 0, 1).Format("2006-01-02")
				if row, ok := byDay[matchDay]; ok && row.RegistrationDay != "" {
					return nil, fail()
				}
			}
			if matchDay >= start.Format("2006-01-02") && matchDay < end.Format("2006-01-02") {
				result = append(result, p13UFCalendarCoverageRow{Entry: entry, MatchDay: matchDay})
			}
		}
		if len(entries) < 128 {
			break
		}
		last := entries[len(entries)-1]
		lastDay, lastGuild = last.Day, last.GuildID
	}
	return result, nil
}

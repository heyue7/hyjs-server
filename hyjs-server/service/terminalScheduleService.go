package service

import (
	"fmt"
	"hyjs-server/model"
	"time"
)

// TerminalScheduleValid validates each row without changing its stable identity.
func TerminalScheduleValid(r model.TerminalWeeklyRule) bool {
	kind := r.Kind
	if kind == "" {
		kind = "weekly"
	}
	clock := r.Time
	if clock == "" {
		clock = "00:00"
	}
	if t, e := time.Parse("15:04", clock); e != nil || t.Format("15:04") != clock {
		return false
	}
	switch kind {
	case "daily":
		return true
	case "weekly":
		return r.Weekday >= 1 && r.Weekday <= 7
	case "monthly":
		return r.Day >= 1 && r.Day <= 31
	case "once":
		t, e := time.Parse("2006-01-02", r.Date)
		return e == nil && t.Format("2006-01-02") == r.Date
	}
	return false
}

// Only the current real Beijing date is eligible. Historical dates are never backfilled.
// Rule identity and period key survive restarts, retries and edits of time/amount.
func terminalScheduleDue(r model.TerminalWeeklyRule, now time.Time) (string, bool) {
	if !r.Enabled || !TerminalScheduleValid(r) {
		return "", false
	}
	local := now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	clock := r.Time
	if clock == "" {
		clock = "00:00"
	}
	if local.Format("15:04") < clock {
		return "", false
	}
	kind := r.Kind
	if kind == "" {
		kind = "weekly"
	}
	switch kind {
	case "daily":
		return "daily-" + terminalDay(now) + "-" + r.ID, true
	case "weekly":
		weekday := int64(local.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if weekday != r.Weekday {
			return "", false
		}
		return "weekly-" + terminalDay(local.AddDate(0, 0, -int(weekday-1))) + "-" + r.ID, true
	case "monthly":
		if int64(local.Day()) != r.Day {
			return "", false
		}
		return "monthly-" + local.Format("2006-01") + "-" + r.ID, true
	case "once":
		if local.Format("2006-01-02") != r.Date {
			return "", false
		}
		return fmt.Sprintf("once-%s", r.ID), true
	}
	return "", false
}

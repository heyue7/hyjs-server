package service

import (
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// The life-day cache contains no boss claims, receipts, battle or wall-clock lease.
type fieldCalendarQuota75 struct {
	Day   string `json:"day"`
	Tired int32  `json:"tired"`
	Max   int32  `json:"max"`
	Kills int32  `json:"kills"`
}

func fieldCalendarDayValid75(day string) bool {
	parsed, err := time.Parse("2006-01-02", day)
	return err == nil && parsed.Format("2006-01-02") == day
}
func (q fieldCalendarQuota75) valid() bool {
	return fieldCalendarDayValid75(q.Day) && q.Max >= 0 && q.Tired >= 0 && q.Tired <= q.Max && q.Kills >= 0
}
func fieldCalendarSnapshot75(g *model.GameState, day string) (fieldCalendarQuota75, error) {
	q := fieldCalendarQuota75{Day: day, Tired: g.Field.Side.GetTiredness(), Max: g.Field.Side.GetMaxTiredness(), Kills: g.Field.RewardKills}
	if !q.valid() {
		return q, errors.New(constUtil.CalendarQuota75Invalid)
	}
	return q, nil
}

// prepareFieldCalendar75 runs under the existing profile transaction. A legacy
// first read preserves used fatigue/kills, while Admin capture makes a genuine
// subsequent date change reversible. Same-day hot reads perform no queries.
func prepareFieldCalendar75(tx *gorm.DB, id int64, g *model.GameState, targetDay string) (bool, error) {
	if g == nil || g.Field == nil || g.Field.Side == nil {
		return false, nil
	}
	if g.Field.RewardDay == targetDay {
		if !g.Field.LifeCalendar75 {
			g.Field.LifeCalendar75 = true
			return true, nil
		}
		return false, nil
	}
	if id < 1 || !fieldCalendarDayValid75(targetDay) {
		return false, errors.New(constUtil.CalendarQuota75Invalid)
	}
	var current struct {
		Day string `json:"day"`
	}
	found, err := calendarQuota75Load(tx, id, constUtil.CalendarQuota75FieldLife, "current", &current)
	if err != nil {
		return false, err
	}
	var target fieldCalendarQuota75
	if !found && !g.Field.LifeCalendar75 {
		target, err = fieldCalendarSnapshot75(g, targetDay)
		if err != nil {
			return false, err
		}
	} else {
		if found && !fieldCalendarDayValid75(current.Day) {
			return false, errors.New(constUtil.CalendarQuota75Invalid)
		}
		// The persisted profile date owns its latest consumption. Admin may
		// advance a marker on an unread date; that marker must not relabel
		// this old profile or hide new consumption after a same-day rewind.
		old, err := fieldCalendarSnapshot75(g, g.Field.RewardDay)
		if err != nil {
			return false, err
		}
		if err = calendarQuota75Save(tx, id, constUtil.CalendarQuota75FieldLife, old.Day, old); err != nil {
			return false, err
		}

		have, err := calendarQuota75Load(tx, id, constUtil.CalendarQuota75FieldLife, targetDay, &target)
		if err != nil {
			return false, err
		}
		if !have {
			target = fieldCalendarQuota75{Day: targetDay, Tired: old.Max, Max: old.Max}
		}
		if !target.valid() || target.Day != targetDay {
			return false, errors.New(constUtil.CalendarQuota75Invalid)
		}
	}
	if err = calendarQuota75Save(tx, id, constUtil.CalendarQuota75FieldLife, targetDay, target); err != nil {
		return false, err
	}
	current.Day = targetDay
	if err = calendarQuota75Save(tx, id, constUtil.CalendarQuota75FieldLife, "current", current); err != nil {
		return false, err
	}
	g.Field.RewardDay, g.Field.RewardKills = target.Day, target.Kills
	g.Field.LifeCalendar75 = true
	g.Field.Side.Tiredness = proto.Int32(target.Tired)
	g.Field.Side.MaxTiredness = proto.Int32(target.Max)
	g.Field.Side.TiredNessLastRefreshDate = proto.String(target.Day)
	return true, nil
}

// captureFieldCalendar75BeforeShift observes profiles already locked by Admin.
// It archives their current real quota under the game date being left without
// rewriting profiles or assigning any new daily resources.
func captureFieldCalendar75BeforeShift(tx *gorm.DB, rows []db.Profile, oldGameTime time.Time) error {
	day := p1Day(oldGameTime)
	if !fieldCalendarDayValid75(day) {
		return errors.New(constUtil.CalendarQuota75Invalid)
	}
	for _, row := range rows {
		if row.Game == nil {
			continue
		}
		var g model.GameState
		if err := json.Unmarshal([]byte(*row.Game), &g); err != nil {
			return err
		}
		if g.Field == nil || g.Field.Side == nil {
			continue
		}
		// Work on a detached JSON decode only: Admin never saves this role.
		if g.Field.RewardDay != day {
			if _, err := prepareFieldCalendar75(tx, row.Id, &g, day); err != nil {
				return err
			}
		}
		q, err := fieldCalendarSnapshot75(&g, day)
		if err != nil {
			return err
		}
		if err = calendarQuota75Save(tx, row.Id, constUtil.CalendarQuota75FieldLife, day, q); err != nil {
			return err
		}
		if err = calendarQuota75Save(tx, row.Id, constUtil.CalendarQuota75FieldLife, "current", struct {
			Day string `json:"day"`
		}{Day: day}); err != nil {
			return err
		}
	}
	return nil
}

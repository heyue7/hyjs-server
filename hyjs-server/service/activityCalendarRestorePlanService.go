package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
)

// ActivityCalendarRestoreWindow is a proposal only; identities and payloads are never replaced.
type ActivityCalendarRestoreWindow struct {
	OccurrenceID  string   `json:"occurrenceId"`
	ConsumerID    string   `json:"consumerId"`
	PlayerID      int64    `json:"playerId,omitempty"`
	Mode          string   `json:"mode"`
	SourceVersion string   `json:"sourceVersion"`
	Before        [3]int64 `json:"before"`
	After         [3]int64 `json:"after"`
	StartDay      int64    `json:"startDay"`
	DurationDays  int64    `json:"durationDays"`
	ClaimDays     int64    `json:"claimDays"`
}

// ActivityCalendarRestorePlan captures a repeatable read-only candidate and its write blockers.
// A digest is evidence for a future CAS operation, never authorization to bypass frozen rules.
type ActivityCalendarRestorePlan struct {
	ReadOnly         bool                                `json:"readOnly"`
	ServerID         int64                               `json:"serverId"`
	OpeningAt        int64                               `json:"openingAt"`
	ClockRevision    int64                               `json:"clockRevision"`
	ClockDeltaDays   int64                               `json:"clockDeltaDays"`
	ScheduleRevision int64                               `json:"scheduleRevision"`
	ServerRevision   int64                               `json:"serverRevision"`
	Digest           string                              `json:"digest"`
	Windows          []ActivityCalendarRestoreWindow     `json:"windows"`
	AnchorPlayerIDs  []int64                             `json:"anchorPlayerIds"`
	Blockers         []string                            `json:"blockers"`
	Dependencies     []ActivityCalendarRestoreDependency `json:"dependencies"`
	Manifest         []ActivityCalendarRestoreManifest   `json:"manifest"`
}

// PreviewActivityCalendarRestore reads existing enabled entries without issuing consumers,
// changing clocks, creating audit rows, or running schema migrations. The caller supplies
// a real timestamp so a saved candidate cannot silently move across midnight.
func (s *GameService) PreviewActivityCalendarRestore(ctx context.Context, real time.Time) (*ActivityCalendarRestorePlan, error) {
	var out *ActivityCalendarRestorePlan
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		snapshot, err := s.readActivityCalendarRestore(tx, real)
		if err != nil {
			return err
		}
		out = snapshot.plan
		return nil
	})
	return out, err
}

func activityCalendarRestorePlan(realm db.AdminGameServer, clock db.ServerGameClock, revision db.AdminActivityScheduleRevision, rows []db.AdminActivityOccurrence, issued []db.AdminCharacterActivityInstance, anchors []db.PlayerOpeningCalendar, real time.Time) *ActivityCalendarRestorePlan {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	day := real.In(zone)
	opening := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
	oldOpening := time.UnixMilli(realm.OpenAt).In(zone)
	oldOpening = time.Date(oldOpening.Year(), oldOpening.Month(), oldOpening.Day(), 0, 0, 0, 0, zone)
	out := &ActivityCalendarRestorePlan{ReadOnly: true, ServerID: realm.ID, OpeningAt: opening.UnixMilli(), ClockRevision: clock.Revision, ClockDeltaDays: -clock.OffsetDays, ScheduleRevision: revision.Revision, ServerRevision: realm.Revision, Windows: []ActivityCalendarRestoreWindow{}, AnchorPlayerIDs: []int64{}, Blockers: []string{}}
	// Hash complete source payloads as well as versions, catching edits to rewards,
	// enabled flags, existing consumer dates and frozen instance ownership.
	forms := make([][]byte, 0, len(rows)+len(issued))
	versions := make([]string, 0, len(rows)+len(issued))
	for _, row := range rows {
		forms = append(forms, row.FormJSON)
		versions = append(versions, adminVersion(row))
	}
	for _, row := range issued {
		forms = append(forms, row.FormJSON)
		versions = append(versions, row.SourceVersion)
	}
	out.Digest = p6Digest(adminJSON([]any{realm, clock, revision, rows, issued, forms, versions, anchors, opening.UnixMilli()}))
	for _, a := range anchors {
		out.AnchorPlayerIDs = append(out.AnchorPlayerIDs, a.PlayerID)
	}
	wheels := []db.AdminActivityOccurrence{}
	for _, row := range rows {
		if row.Mode == "turntable" && adminActivityTiming(row) == "character-day" {
			wheels = append(wheels, row)
		}
	}
	sort.Slice(wheels, func(i, j int) bool {
		if wheels[i].StartDay == wheels[j].StartDay {
			return wheels[i].ID < wheels[j].ID
		}
		return wheels[i].StartDay < wheels[j].StartDay
	})
	wheelIndex := map[string]int{}
	for i, row := range wheels {
		wheelIndex[row.ID] = i
	}
	active := map[string]ActivityCalendarRestoreWindow{}
	for _, row := range rows {
		start, duration, grace := adminCharacterScheduleDays(row)
		w := ActivityCalendarRestoreWindow{OccurrenceID: row.ID, ConsumerID: row.ConsumerID, Mode: row.Mode, SourceVersion: adminVersion(row), Before: [3]int64{row.StartAt, row.EndAt, row.ClaimEndAt}, StartDay: start, DurationDays: duration, ClaimDays: grace}
		var form struct {
			Name    string `json:"name"`
			Options struct {
				ShopKey string `json:"shopKey"`
			} `json:"options"`
		}
		if err := json.Unmarshal(row.FormJSON, &form); err != nil {
			out.Blockers = append(out.Blockers, fmt.Sprintf("occurrence %s: invalid FormJSON", row.ID))
			continue
		}
		if adminActivityTiming(row) == "character-day" {
			if row.Mode == "recharge-total" && form.Name == "启程庆典" {
				w.StartDay, w.DurationDays = 1, 3
			}
			if row.Mode == "opening-14" {
				w.DurationDays = 14
			}
			if i, ok := wheelIndex[row.ID]; ok {
				if i == 0 {
					w.StartDay, w.DurationDays = 1, 7
				} else {
					w.StartDay, w.DurationDays = int64(8+(i-1)*14), 14
				}
			}
			at := opening.AddDate(0, 0, int(w.StartDay-1))
			end := at.AddDate(0, 0, int(w.DurationDays))
			w.After = [3]int64{at.UnixMilli(), end.UnixMilli(), end.AddDate(0, 0, int(w.ClaimDays)).UnixMilli()}
		} else {
			shift := opening.UnixMilli() - oldOpening.UnixMilli()
			w.After = [3]int64{row.StartAt + shift, row.EndAt + shift, row.ClaimEndAt + shift}
			old := time.UnixMilli(row.StartAt).In(zone)
			if row.Mode == "recharge-total" && form.Name == "月度盛典" || row.Mode == "shop-window" && form.Options.ShopKey == "guild" {
				monthDelta := int(old.Year()-oldOpening.Year())*12 + int(old.Month()-oldOpening.Month())
				at := time.Date(opening.Year(), opening.Month()+time.Month(monthDelta), old.Day(), 0, 0, 0, 0, zone)
				if at.Day() != old.Day() {
					out.Blockers = append(out.Blockers, fmt.Sprintf("occurrence %s: day %d does not exist in target month; review monthly schedule", row.ID, old.Day()))
					continue
				}
				end := at.Add(time.Duration(row.EndAt-row.StartAt) * time.Millisecond)
				if row.Mode == "shop-window" {
					end = at.AddDate(0, 1, 0)
				}
				w.After = [3]int64{at.UnixMilli(), end.UnixMilli(), end.UnixMilli() + row.ClaimEndAt - row.EndAt}
			}
		}
		active[row.ID] = w
		out.Windows = append(out.Windows, w)
	}
	for _, instance := range issued {
		w, ok := active[instance.OccurrenceID]
		if !ok {
			continue
		}
		w.PlayerID, w.ConsumerID, w.SourceVersion = instance.PlayerID, instance.ConsumerID, instance.SourceVersion
		w.Before = [3]int64{instance.StartAt, instance.EndAt, instance.ClaimEndAt}
		out.Windows = append(out.Windows, w)

	}
	return out
}

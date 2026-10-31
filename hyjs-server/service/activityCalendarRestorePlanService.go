package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	Formal110        bool                                `json:"formal110,omitempty"`
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
	return s.previewActivityCalendarRestore(ctx, real, false)
}

// PreviewFormal110ActivityCalendarRestoreCLI is reserved for the one-time offline
// migration. Its candidate cannot be applied through the normal restore path.
func (s *GameService) PreviewFormal110ActivityCalendarRestoreCLI(ctx context.Context, real time.Time) (*ActivityCalendarRestorePlan, error) {
	return s.previewActivityCalendarRestore(ctx, real, true)
}

func (s *GameService) previewActivityCalendarRestore(ctx context.Context, real time.Time, formal110 bool) (*ActivityCalendarRestorePlan, error) {
	var out *ActivityCalendarRestorePlan
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		snapshot, err := s.readActivityCalendarRestore(tx, real, formal110)
		if err != nil {
			return err
		}
		out = snapshot.plan
		return nil
	})
	return out, err
}

func activityCalendarRestorePlan(realm db.AdminGameServer, clock db.ServerGameClock, revision db.AdminActivityScheduleRevision, rows []db.AdminActivityOccurrence, issued []db.AdminCharacterActivityInstance, anchors []db.PlayerOpeningCalendar, real time.Time) *ActivityCalendarRestorePlan {
	return activityCalendarRestorePlanWithFormal110(realm, clock, revision, rows, issued, anchors, real, false)
}

func activityCalendarRestorePlanWithFormal110(realm db.AdminGameServer, clock db.ServerGameClock, revision db.AdminActivityScheduleRevision, rows []db.AdminActivityOccurrence, issued []db.AdminCharacterActivityInstance, anchors []db.PlayerOpeningCalendar, real time.Time, formal110 bool) *ActivityCalendarRestorePlan {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	day := real.In(zone)
	opening := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
	oldOpening := time.UnixMilli(realm.OpenAt).In(zone)
	oldOpening = time.Date(oldOpening.Year(), oldOpening.Month(), oldOpening.Day(), 0, 0, 0, 0, zone)
	out := &ActivityCalendarRestorePlan{ReadOnly: true, Formal110: formal110, ServerID: realm.ID, OpeningAt: opening.UnixMilli(), ClockRevision: clock.Revision, ClockDeltaDays: -clock.OffsetDays, ScheduleRevision: revision.Revision, ServerRevision: realm.Revision, Windows: []ActivityCalendarRestoreWindow{}, AnchorPlayerIDs: []int64{}, Blockers: []string{}}
	monthlyIndex := map[string]int{}
	if formal110 {
		monthlyIndex = formal110RestoreOccurrences(out, rows, opening)
	}
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
			if row.Mode == "recharge-total" && (form.Name == "启程庆典" || formal110 && row.TemplateID == formal110PersonalTemplate) {
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
			if month, ok := monthlyIndex[row.ID]; formal110 && ok {
				at := time.Date(opening.Year(), opening.Month()+time.Month(month), 15, 0, 0, 0, 0, zone)
				end := at.AddDate(0, 0, 3)
				w.After = [3]int64{at.UnixMilli(), end.UnixMilli(), end.UnixMilli()}
			} else if row.Mode == "recharge-total" && form.Name == "月度盛典" || row.Mode == "shop-window" && form.Options.ShopKey == "guild" {
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
	if formal110 {
		// Include the explicit one-off mode and proposed results in the reviewed CAS.
		out.Digest = p6Digest(adminJSON([]any{"formal-110-v1", out.Digest, out.Windows, out.Blockers}))
	}
	return out
}

const (
	formal110PersonalTemplate = "template_0665a16f723e4e5b9d3febfb"
	formal110MonthlyTemplate  = "template_270c5d9593bb42e5b8e51f7c"
)

func formal110OpeningDate(serverID int64, real time.Time) bool {
	day := real.In(time.FixedZone("Asia/Shanghai", 8*3600))
	return serverID == 1 && day.Year() == 2026 && day.Month() == time.November && day.Day() == 1
}

// formal110RestoreOccurrences matches the approved publication by stable template
// and consumer identity. Names are normalized and cannot identify these schedules.
func formal110RestoreOccurrences(plan *ActivityCalendarRestorePlan, rows []db.AdminActivityOccurrence, opening time.Time) map[string]int {
	if !formal110OpeningDate(plan.ServerID, opening) {
		plan.Blockers = append(plan.Blockers, "formal110: requires server 1 and Beijing opening date 2026-11-01")
	}
	var personal []db.AdminActivityOccurrence
	var monthly []db.AdminActivityOccurrence
	for _, row := range rows {
		if row.Mode == "stamina-meal" {
			plan.Blockers = append(plan.Blockers, "formal110: disable restaurant before restoring; preserve its window")
		}
		if row.TemplateID != formal110PersonalTemplate && row.TemplateID != formal110MonthlyTemplate {
			continue
		}
		var form struct {
			Alias      string `json:"alias"`
			Source     string `json:"source"`
			TemplateID string `json:"templateId"`
		}
		if json.Unmarshal(row.FormJSON, &form) != nil || form.Alias != "成长投放V12" || form.Source != "成长投放V12 / 2026-10-26" || form.TemplateID != "recharge-total-original" || row.ServerID != 1 || !row.Enabled || row.Archived || row.Mode != "recharge-total" || row.ID == "" {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("formal110: occurrence %s has unexpected publication identity", row.ID))
		}
		if row.TemplateID == formal110PersonalTemplate {
			personal = append(personal, row)
			if row.Timing != "character-day" || row.StartDay != 1 || (row.DurationDays != 14 && row.DurationDays != 3) || row.ClaimDays != 0 || !strings.HasPrefix(row.ConsumerKey, "character_plan_") || row.ConsumerID != "character-plan:"+row.ConsumerKey {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("formal110: occurrence %s has unexpected personal consumer", row.ID))
			}
		} else {
			monthly = append(monthly, row)
			if row.Timing != "calendar" || row.DurationDays != 3 || row.ClaimDays != 0 || !strings.HasPrefix(row.ConsumerKey, "rechargeTotal_p4_admin_schedule_") || row.ConsumerID != "period:"+row.ConsumerKey || row.StartAt <= 0 || row.EndAt-row.StartAt != 3*86400000 || row.ClaimEndAt != row.EndAt {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("formal110: occurrence %s has unexpected monthly consumer", row.ID))
			}
		}
	}
	if len(personal) != 1 || len(monthly) != 7 {
		plan.Blockers = append(plan.Blockers, fmt.Sprintf("formal110: expected one personal and seven monthly occurrences; found %d and %d", len(personal), len(monthly)))
	}
	sort.Slice(monthly, func(i, j int) bool {
		if monthly[i].StartAt == monthly[j].StartAt {
			return monthly[i].ID < monthly[j].ID
		}
		return monthly[i].StartAt < monthly[j].StartAt
	})
	index := make(map[string]int, len(monthly))
	consumers := map[string]bool{}
	for i, row := range monthly {
		if (i > 0 && row.StartAt == monthly[i-1].StartAt) || consumers[row.ConsumerID] {
			plan.Blockers = append(plan.Blockers, "formal110: duplicate monthly start or consumer")
		}
		if _, exists := index[row.ID]; exists {
			plan.Blockers = append(plan.Blockers, "formal110: duplicate monthly occurrence")
		}
		consumers[row.ConsumerID] = true
		index[row.ID] = i
	}
	return index
}

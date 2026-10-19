package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func adminActivityTiming(row db.AdminActivityOccurrence) string {
	if row.Mode == "opening-14" || row.Timing == "character-day" {
		return "character-day"
	}
	return "calendar"
}
func adminCharacterScheduleDays(row db.AdminActivityOccurrence) (int64, int64, int64) {
	if row.Mode == "opening-14" && row.Timing != "character-day" {
		return 1, 14, max(int64(0), (row.ClaimEndAt-row.EndAt+86399999)/86400000)
	}
	start, duration, grace := row.StartDay, row.DurationDays, row.ClaimDays
	if start == 0 {
		start = 1
	}
	if duration == 0 {
		duration = 14
	}
	return start, duration, grace
}
func adminNormalizeCharacterItems(items []model.AdminActivityOccurrenceDTO) []model.AdminActivityOccurrenceDTO {
	out := append([]model.AdminActivityOccurrenceDTO{}, items...)
	for index := range out {
		item := &out[index]
		if item.Mode == "opening-14" {
			item.Timing = "character-day"
		}
		if item.Timing == "" {
			item.Timing = "calendar"
		}
		if item.Timing == "character-day" && item.StartDay == 0 {
			item.StartDay = 1
		}
		if item.Timing == "character-day" && item.DurationDays == 0 {
			item.DurationDays = 14
			if item.Mode == "opening-14" {
				item.ClaimDays = 1
			}
		}
	}
	return out
}
func adminActivityWindowsOverlap(a, b model.AdminActivityOccurrenceDTO, mode string) bool {

	if a.Timing != b.Timing {
		return true
	}
	if a.Timing == "character-day" {
		return a.StartDay < b.StartDay+b.DurationDays+b.ClaimDays && b.StartDay < a.StartDay+a.DurationDays+a.ClaimDays
	}
	return a.StartAt < b.ClaimEndAt && b.StartAt < a.ClaimEndAt
}

// PrepareAdminCharacterActivitySchedules runs after original authentication and
// trusted realm/calendar binding. It serializes clock -> profile -> schedule,
// materializes only the role's eligible windows, and never rewrites an issued
// native consumer or participates in the retired local-demo tool permissions.
func (s *GameService) PrepareAdminCharacterActivitySchedules(ctx context.Context, playerID int64) error {
	d := db.GetIns().WithContext(ctx)
	if playerID < 1 || !d.Migrator().HasTable(&db.AdminCharacterActivityInstance{}) {
		return nil
	}
	var count int64
	if err := d.Model(&db.AdminActivityOccurrence{}).Where("server_id=? AND timing=? AND mode<>? AND enabled=? AND archived=?", s.OriginalServerID(), "character-day", "opening-14", true, false).Count(&count).Error; err != nil || count == 0 {
		return err
	}
	if GameCalendarOwner(ctx) != playerID {
		return constUtil.ErrAdminInput
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, playerID).Error; err != nil {
			return err
		}
		if AdminProfileServerID(profile) != s.OriginalServerID() || profile.Player == nil {
			return constUtil.ErrAdminInput
		}
		// Keep the established profile -> P1 windows -> activity source order.
		// MySQL's period range lock also serializes insertion of a new native
		// cumulative consumer against a concurrent calendar publication.
		if err := lockProfilePeriods(tx); err != nil {
			return err
		}
		var anchor db.PlayerOpeningCalendar
		err := tx.First(&anchor, "player_id=?", playerID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if profile.CreatedAt == nil || *profile.CreatedAt <= 0 {
				return constUtil.ErrAdminConfiguration
			}
			zone := time.FixedZone("UTC+8", 8*3600)
			day := time.UnixMilli(*profile.CreatedAt).In(zone)
			anchor = db.PlayerOpeningCalendar{PlayerID: playerID, StartAt: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone).UnixMilli(), CreatedAt: time.Now().UnixMilli(), Source: constUtil.PlayerOpeningCalendarSource}
			if err = tx.Create(&anchor).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if anchor.Source != constUtil.PlayerOpeningCalendarSource || anchor.StartAt <= 0 {
			return constUtil.ErrAdminConfiguration
		}
		var schedules []db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("server_id=? AND timing=? AND mode<>? AND enabled=? AND archived=?", s.OriginalServerID(), "character-day", "opening-14", true, false).Order("id").Find(&schedules).Error; err != nil {
			return err
		}
		now := LocalPlayNow(ctx).UnixMilli()
		for _, schedule := range schedules {
			start, duration, grace := adminCharacterScheduleDays(schedule)
			if start < 1 || duration < 1 || start > 366 || duration > 366 || grace < 0 || grace > 90 {
				return constUtil.ErrAdminConfiguration
			}
			startAt := anchor.StartAt + (start-1)*86400000
			endAt := startAt + duration*86400000
			claimEndAt := endAt + grace*86400000
			if now < startAt || now >= claimEndAt {
				continue
			}
			var issued db.AdminCharacterActivityInstance
			err := tx.First(&issued, "player_id=? AND occurrence_id=?", playerID, schedule.ID).Error
			if err == nil {
				accepted := schedule
				accepted.FormJSON = issued.FormJSON
				if _, e := adminAcceptActivityTemplate(tx, playerID, accepted); e != nil {
					return e
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			allowed, err := adminAcceptActivityTemplate(tx, playerID, schedule)
			if err != nil {
				return err
			}
			if !allowed {
				continue
			}
			definition, ok := adminActivityDefinitionFor(schedule.Mode)
			if !ok {
				return constUtil.ErrAdminConfiguration
			}
			var form model.AdminActivityForm
			if json.Unmarshal(schedule.FormJSON, &form) != nil {
				return constUtil.ErrAdminConfiguration
			}
			form.StartAt = startAt
			form.EndAt = endAt
			form.ClaimEndAt = claimEndAt
			digest := p6Digest([]byte(fmt.Sprintf("%d/%d/%s", s.OriginalServerID(), playerID, schedule.ID)))[:20]
			key := definition.prefix + "character_" + digest
			if form.Mode == "shop-window" {
				key = "shop_schedule_" + form.Options.ShopKey + "_character_" + digest
			}
			if err := s.adminPersistTypedActivity(tx, definition, key, form, true, false); err != nil {
				return err
			}
			issued = db.AdminCharacterActivityInstance{PlayerID: playerID, OccurrenceID: schedule.ID, ServerID: s.OriginalServerID(), ConsumerID: definition.domain + ":" + key, ConsumerKey: key, Mode: schedule.Mode, StartAt: startAt, EndAt: endAt, ClaimEndAt: claimEndAt, SourceVersion: adminVersion(schedule), FormJSON: adminJSON(form)}
			if form.Mode == "shop-window" && !adminOrdinarySchedulableShop(form.Options.ShopKey) {
				issued.ConsumerID = "shop-gate:" + key
			}
			if err := tx.Create(&issued).Error; err != nil {
				return err
			}
			meta := db.AdminActivityPresentation{ActivityID: issued.ConsumerID, Mode: schedule.Mode, TemplateID: form.TemplateID, Name: schedule.Name, UpdatedAtMillis: time.Now().UnixMilli()}
			if err := tx.Create(&meta).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// adminCharacterActivityAccess retains historical receipts but prevents a role
// from guessing another role's native instance or using a disabled realm plan.
func adminCharacterActivityAccess(tx *gorm.DB, key string) (bool, error) {
	if !tx.Migrator().HasTable(&db.AdminCharacterActivityInstance{}) {
		return false, nil
	}
	owner := GameCalendarOwner(tx.Statement.Context)
	service, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService)
	if !ok || owner < 1 {
		return false, nil
	}
	var issued db.AdminCharacterActivityInstance
	err := tx.Where("consumer_key=?", key).Take(&issued).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if issued.PlayerID != owner || issued.ServerID != service.OriginalServerID() {
		return false, nil
	}
	var occurrence db.AdminActivityOccurrence
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&occurrence, "id=?", issued.OccurrenceID).Error; err != nil {
		return false, err
	}
	if occurrence.ServerID != issued.ServerID || adminActivityTiming(occurrence) != "character-day" {
		return false, nil
	}
	accepted := occurrence
	accepted.FormJSON = issued.FormJSON
	allowed, err := adminAcceptActivityTemplate(tx, owner, accepted)
	if err != nil {
		return false, err
	}
	return allowed && occurrence.Enabled && !occurrence.Archived, nil
}
func adminCharacterActivityCurrentKey(tx *gorm.DB, mode string, now int64, claim bool) (string, error) {
	if !tx.Migrator().HasTable(&db.AdminCharacterActivityInstance{}) {
		return "", nil
	}
	owner := GameCalendarOwner(tx.Statement.Context)
	service, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService)
	if !ok || owner < 1 {
		return "", nil
	}
	end := "end_at"
	if claim {
		end = "claim_end_at"
	}
	var rows []db.AdminCharacterActivityInstance
	if err := tx.Where("player_id=? AND server_id=? AND mode=? AND start_at<=? AND "+end+">?", owner, service.OriginalServerID(), mode, now, now).Order("start_at DESC,occurrence_id").Find(&rows).Error; err != nil {
		return "", err
	}
	found := ""
	for _, row := range rows {
		allowed, err := adminCharacterActivityAccess(tx, row.ConsumerKey)
		if err != nil {
			return "", err
		}
		if allowed {
			if found != "" {
				return "", constUtil.ErrAdminConfiguration
			}
			found = row.ConsumerKey
		}
	}
	return found, nil
}

// AdminCurrentCharacterWelfareRuleKey narrows original no-instance welfare to
// the authenticated role's active consumer; old global pending-order logic is
// unchanged when no role-specific window exists.
func AdminCurrentCharacterWelfareRuleKey(tx *gorm.DB, kind string, now int64) (string, error) {
	for _, definition := range adminActivityDefinitions() {
		if definition.domain == "welfare" && definition.kind == kind {
			return adminCharacterActivityCurrentKey(tx, definition.mode, now, true)
		}
	}
	return "", nil
}

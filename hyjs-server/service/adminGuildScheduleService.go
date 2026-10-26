package service

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strings"
	"time"
)

func adminGuildScheduleFrozen(tx *gorm.DB, row *db.P13UnionFightSchedule) (bool, error) {
	registration := row.RegistrationDay
	if registration == "" {
		var err error
		registration, err = p13UFRegistrationDay(tx, row.Day)
		if err != nil {
			return false, err
		}
	}
	var count int64
	if err := tx.Model(&db.P13UnionFightEntry{}).Where("day=?", registration).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := tx.Model(&db.P13UnionFightMatch{}).Where("day=?", row.Day).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func adminGuildScheduleProjection(row db.P13UnionFightSchedule, now time.Time, frozen bool) model.AdminGuildScheduleRow {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	civil := func(at int64, layout string) string {
		if at <= 0 {
			return ""
		}
		return time.UnixMilli(at).In(zone).Format(layout)
	}
	out := model.AdminGuildScheduleRow{AdminGuildScheduleForm: model.AdminGuildScheduleForm{Day: row.Day, EnrollmentStart: civil(row.EnrollmentStartAt, "2006-01-02 15:04"), EnrollmentEnd: civil(row.EnrollmentEndAt, "2006-01-02 15:04"), FirstStartTime: civil(row.FirstStartAt, "15:04"), SecondStartTime: civil(row.SecondStartAt, "15:04"), DurationMinutes: row.DurationMillis / 60000, Enabled: row.Enabled, Version: row.Version}, RegistrationDay: row.RegistrationDay, FirstEnd: civil(row.FirstStartAt+row.DurationMillis, "2006-01-02 15:04"), SecondEnd: civil(row.SecondStartAt+row.DurationMillis, "2006-01-02 15:04"), PublishedAt: row.PublishedAt, RowVersion: adminVersion(row), Legacy: row.RegistrationDay == ""}
	out.Frozen = frozen
	out.Editable = !out.Frozen && !out.Legacy && row.EnrollmentStartAt > now.UnixMilli()
	return out
}

func (s *GameService) AdminGuildSchedules(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminGuildSchedulesResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	var policy db.P13UnionFightPolicy
	var field db.P13UnionFightFieldPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	if err := tx.First(&field, 1).Error; err != nil {
		return nil, err
	}
	out := &model.AdminGuildSchedulesResponse{Items: []model.AdminGuildScheduleRow{}, CustomCalendarEnabled: policy.CustomCalendarEnabled, PolicyVersion: adminVersion(policy), PreparationMinutes: float64(field.PreparationMillis) / 60000}
	var rows []db.P13UnionFightSchedule
	if err := tx.Order("day DESC").Limit(128).Find(&rows).Error; err != nil {
		return nil, err
	}
	registrationDays, matchDays := []string{}, []string{}
	for _, row := range rows {
		day := row.RegistrationDay
		if day == "" {
			parsed, err := time.Parse("2006-01-02", row.Day)
			if err != nil {
				return nil, err
			}
			day = parsed.AddDate(0, 0, -1).Format("2006-01-02")
		}
		registrationDays = append(registrationDays, day)
		matchDays = append(matchDays, row.Day)
	}
	var entryDays, createdDays []string
	if len(rows) > 0 {
		if err := tx.Model(&db.P13UnionFightEntry{}).Distinct("day").Where("day IN ?", registrationDays).Pluck("day", &entryDays).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&db.P13UnionFightMatch{}).Distinct("day").Where("day IN ?", matchDays).Pluck("day", &createdDays).Error; err != nil {
			return nil, err
		}
	}
	entries, matches := map[string]bool{}, map[string]bool{}
	for _, day := range entryDays {
		entries[day] = true
	}
	for _, day := range createdDays {
		matches[day] = true
	}
	for i, row := range rows {
		out.Items = append(out.Items, adminGuildScheduleProjection(row, LocalPlayNow(ctx), entries[registrationDays[i]] || matches[row.Day]))
	}
	return out, nil
}

func adminGuildScheduleValue(req model.AdminGuildScheduleSaveRequest, now time.Time, preparation int64) (*db.P13UnionFightSchedule, error) {
	if strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 || strings.TrimSpace(req.Version) != req.Version || req.Version == "" || len(req.Version) > 64 {
		return nil, constUtil.ErrAdminInput
	}
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	parse := func(layout, value string) (time.Time, error) {
		at, err := time.ParseInLocation(layout, value, zone)
		if err == nil && at.Format(layout) != value {
			return at, constUtil.ErrAdminInput
		}
		return at, err
	}
	start, e1 := parse("2006-01-02 15:04", req.EnrollmentStart)
	end, e2 := parse("2006-01-02 15:04", req.EnrollmentEnd)
	first, e3 := parse("2006-01-02 15:04", req.Day+" "+req.FirstStartTime)
	second, e4 := parse("2006-01-02 15:04", req.Day+" "+req.SecondStartTime)
	minutes := req.DurationMinutes
	if minutes == 0 {
		minutes = 30
	}
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || minutes < 1 || minutes > 30 || start.UnixMilli() <= now.UnixMilli() {
		return nil, constUtil.ErrAdminInput
	}
	row := &db.P13UnionFightSchedule{PreparationMillis: preparation, Day: req.Day, RegistrationDay: start.Format("2006-01-02"), EnrollmentStartAt: start.UnixMilli(), EnrollmentEndAt: end.UnixMilli(), FirstStartAt: first.UnixMilli(), SecondStartAt: second.UnixMilli(), DurationMillis: minutes * 60000, Enabled: req.Enabled, Version: req.Version, PublishedAt: now.UnixMilli(), Source: "后台明确发布公会战排期；北京时间；版本：" + req.Version + "；原因：" + req.Reason}
	if err := p13UFCustomScheduleValid(row, preparation); err != nil {
		return nil, constUtil.ErrAdminInput
	}
	return row, nil
}

func (s *GameService) AdminSaveGuildSchedule(ctx context.Context, actor *model.AdminPrincipal, req model.AdminGuildScheduleSaveRequest, create bool) (*model.AdminGuildScheduleRow, error) {
	now := LocalPlayNow(ctx)
	var out model.AdminGuildScheduleRow
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var field db.P13UnionFightFieldPolicy
		if err := tx.First(&field, 1).Error; err != nil {
			return err
		}
		preparation := field.PreparationMillis
		if req.Enabled && preparation == 0 && p13FunctionalDefault(&field) {
			preparation = constUtil.P13UFFunctionalPreparationMillis
		}
		row, err := adminGuildScheduleValue(req, now, preparation)
		if err != nil {
			return err
		}
		var prior db.P13UnionFightSchedule
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&prior, "day=?", row.Day).Error
		if create {
			if req.RowVersion != "" || err == nil {
				return constUtil.ErrAdminConfigConflict
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			if adminVersion(prior) != req.RowVersion || prior.RegistrationDay == "" || prior.EnrollmentStartAt <= now.UnixMilli() {
				return constUtil.ErrAdminConfigConflict
			}
			frozen, err := adminGuildScheduleFrozen(tx, &prior)
			if err != nil {
				return err
			}
			if frozen {
				return constUtil.ErrAdminConfigConflict
			}
		}
		frozen, err := adminGuildScheduleFrozen(tx, row)
		if err != nil {
			return err
		}
		if frozen {
			return constUtil.ErrAdminConfigConflict
		}
		var count int64
		if err = tx.Model(&db.P13UnionFightSchedule{}).Where("day<>? AND registration_day=?", row.Day, row.RegistrationDay).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return constUtil.ErrAdminConfigConflict
		}
		if row.Enabled {
			if err = tx.Model(&db.P13UnionFightSchedule{}).Where("day<>? AND enabled=? AND registration_day<>'' AND enrollment_start_at<? AND second_start_at+duration_millis>?", row.Day, true, row.SecondStartAt+row.DurationMillis, row.EnrollmentStartAt).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return constUtil.ErrAdminConfigConflict
			}
		}
		var summary *P13UFFunctionalRulesSummary
		if row.Enabled {
			if summary, err = s.PrepareP13UnionFightFunctionalRules(tx, row, now); err != nil {
				return err
			}
		}
		if err = tx.Save(row).Error; err != nil {
			return err
		}
		out = adminGuildScheduleProjection(*row, now, false)
		detail := req.Reason + " | " + req.RowVersion + " -> " + out.RowVersion
		if summary != nil {
			out.PublicationNotes = append(out.PublicationNotes, summary.Warnings...)
			for _, period := range summary.Monthly {
				if period.Status == "month-started-no-retro-policy-or-offer" {
					out.PublicationNotes = append(out.PublicationNotes, period.Period+" 已开始，月奖不追溯发放。")
				}
			}
			for _, period := range summary.Weekly {
				if period.Status == "first-match-started-no-retro-offer" {
					out.PublicationNotes = append(out.PublicationNotes, period.Period+" 周已有比赛开战，保留原周奖状态。")
					break
				}
			}
			raw, err := json.Marshal(summary)
			if err != nil {
				return err
			}
			detail += " | " + string(raw)
		}
		return adminAudit(tx, actor, "guild.schedule.save", row.Day, detail)
	})
	return &out, err
}

func (s *GameService) AdminSaveGuildCalendar(ctx context.Context, actor *model.AdminPrincipal, req model.AdminGuildCalendarSaveRequest) (*model.AdminGuildSchedulesResponse, error) {
	if strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 || len(req.RowVersion) != 64 {
		return nil, constUtil.ErrAdminInput
	}
	now := LocalPlayNow(ctx)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var policy db.P13UnionFightPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if adminVersion(policy) != req.RowVersion {
			return constUtil.ErrAdminConfigConflict
		}
		if policy.CustomCalendarEnabled != req.Enabled {
			var count int64
			pendingCalendar := tx.Model(&db.P13UnionFightSchedule{}).Select("1").Where("day>=? AND second_start_at+duration_millis>?", now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02"), now.UnixMilli()).Where("(registration_day<>'' AND registration_day=p13_union_fight_entries.day) OR (registration_day='' AND first_start_at>=p13_union_fight_entries.deadline AND first_start_at<p13_union_fight_entries.deadline+86400000)")
			if err := tx.Model(&db.P13UnionFightEntry{}).Where("deadline>? OR (match_processed=? AND EXISTS (?))", now.UnixMilli(), false, pendingCalendar).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return constUtil.ErrAdminConfigConflict
			}
			if err := tx.Model(&db.P13UnionFightMatch{}).Where("end_at>?", now.UnixMilli()).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return constUtil.ErrAdminConfigConflict
			}
		}
		policy.CustomCalendarEnabled = req.Enabled
		if err := tx.Model(&policy).Update("custom_calendar_enabled", req.Enabled).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, "guild.calendar.save", "1", req.Reason+" | "+req.RowVersion+" -> "+adminVersion(policy))
	})
	if err != nil {
		return nil, err
	}
	return s.AdminGuildSchedules(ctx, actor)
}

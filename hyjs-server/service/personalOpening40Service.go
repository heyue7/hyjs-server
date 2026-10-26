package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func personalOpening40Mode(mode string) bool {
	switch mode {
	case "login-total", "single-spend", "arena-wins", "plunder-count", "wanted-attempts", "wanted-kills", "copy-win", "diamond-spent", "work":
		return true
	}
	return false
}
func personalOpening40ID(mode string, server int64) string {
	return "restore_re1051_" + strings.ReplaceAll(mode, "-", "_") + "_" + strconv.FormatInt(server, 10)
}
func personalOpening40Source(mode string, server int64) string {
	d, ok := adminActivityDefinitionFor(mode)
	if !ok {
		return ""
	}
	return d.prefix + "re1051_schedule_" + strconv.FormatInt(server, 10)
}
func personalOpening40Rule(row db.AdminActivityOccurrence) bool {
	return personalOpening40Mode(row.Mode) && row.ID == personalOpening40ID(row.Mode, row.ServerID) && row.ConsumerKey == personalOpening40Source(row.Mode, row.ServerID) && row.Timing == "character-day"
}

// InitializePersonalOpening40 converts only the audited Re bootstrap identities.
// It preserves old ledgers and serializes with realm scheduling before changing visibility.
func (s *GameService) InitializePersonalOpening40(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		server := s.OriginalServerID()
		revision, err := adminActivityScheduleRevision(tx, server, true)
		if err != nil {
			return err
		}
		target := constUtil.PersonalOpening40TargetPrefix + strconv.FormatInt(server, 10)
		var applied int64
		if err = tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.PersonalOpening40MigrateAction, target).Count(&applied).Error; err != nil || applied > 0 {
			return err
		}
		var rows []db.AdminActivityOccurrence
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=?", server).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		originals := []db.AdminActivityOccurrence{}
		periods := []db.PeriodInstance{}
		for _, row := range rows {
			if !personalOpening40Mode(row.Mode) || row.ID != personalOpening40ID(row.Mode, server) {
				continue
			}
			if row.ConsumerKey != personalOpening40Source(row.Mode, server) {
				return constUtil.ErrAdminConfiguration
			}
			originals = append(originals, row)
			var period db.PeriodInstance
			if err = tx.First(&period, "id=?", row.ConsumerKey).Error; err != nil {
				return err
			}
			periods = append(periods, period)
		}
		actor := &model.AdminPrincipal{Username: "system"}
		forms := map[string][]byte{}
		frozen := map[string][]byte{}
		for _, row := range originals {
			forms[row.ID] = append([]byte{}, row.FormJSON...)
		}
		for _, row := range periods {
			frozen[row.ID] = append([]byte{}, row.Frozen...)
		}
		backup := adminJSON(struct {
			Occurrences []db.AdminActivityOccurrence `json:"occurrences"`
			Periods     []db.PeriodInstance          `json:"periods"`
			Forms       map[string][]byte            `json:"forms"`
			Frozen      map[string][]byte            `json:"frozen"`
		}{originals, periods, forms, frozen})
		// The backup is in the same transaction, so a failed conversion leaves no partial migration marker.
		if err = adminAudit(tx, actor, constUtil.PersonalOpening40BackupAction, target, string(backup)); err != nil {
			return err
		}
		for _, row := range originals {
			var definition db.AdminActivityDefinition
			if err = tx.First(&definition, "id=?", row.TemplateID).Error; err != nil {
				return err
			}
			if definition.DefaultDays < 1 || definition.DefaultDays > 366 || definition.ClaimDays < 0 || definition.ClaimDays > 90 {
				return constUtil.ErrAdminConfiguration
			}
			if row.Timing != "character-day" {
				if err = tx.Model(&row).Updates(map[string]any{"timing": "character-day", "start_day": 1, "duration_days": definition.DefaultDays, "claim_days": definition.ClaimDays, "revision": row.Revision + 1}).Error; err != nil {
					return err
				}
			}
			if err = tx.Model(&db.PeriodInstance{}).Where("id=?", row.ConsumerKey).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		revision.Revision++
		if err = tx.Save(&revision).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, constUtil.PersonalOpening40MigrateAction, target, fmt.Sprintf("%d项个人活动按角色创建日开放；旧进度与领取来源保留，过期角色不补开", len(originals)))
	})
}

// Bind only freshly created owned instances of the exact converted bootstrap rule.
func personalOpening40BindSource(tx *gorm.DB, source db.AdminActivityOccurrence, issued db.AdminCharacterActivityInstance) error {
	if !personalOpening40Rule(source) {
		return nil
	}
	var cfg db.PeriodInstance
	if err := tx.First(&cfg, "id=?", issued.ConsumerKey).Error; err != nil {
		return err
	}
	if len(cfg.Frozen) > 0 || cfg.RewardSourceID != "" {
		return constUtil.ErrAdminConfiguration
	}
	return tx.Model(&cfg).Update("reward_source_id", source.ConsumerKey).Error
}

// Validate the persisted ownership chain rather than trusting a frozen JSON field alone.
func personalOpening40ValidateSource(tx *gorm.DB, cfg db.PeriodInstance, playerID int64) error {
	if cfg.RewardSourceID == "" {
		return nil
	}
	var issued db.AdminCharacterActivityInstance
	if err := tx.Where("consumer_key=?", cfg.ID).Take(&issued).Error; err != nil {
		return constUtil.ErrAdminConfiguration
	}
	if playerID > 0 && issued.PlayerID != playerID {
		return constUtil.ErrAdminConfiguration
	}
	var source db.AdminActivityOccurrence
	if err := tx.First(&source, "id=?", issued.OccurrenceID).Error; err != nil {
		return err
	}
	if !personalOpening40Rule(source) || issued.Mode != source.Mode || issued.ServerID != source.ServerID || cfg.RewardSourceID != source.ConsumerKey || cfg.StartAt != issued.StartAt || cfg.EndAt != issued.EndAt || cfg.RewardEndAt != issued.ClaimEndAt {
		return constUtil.ErrAdminConfiguration
	}
	return nil
}
func personalOpening40InheritProgress(tx *gorm.DB, progress *db.PeriodProgress, cfg db.PeriodInstance) error {
	if cfg.RewardSourceID == "" {
		return nil
	}
	if err := personalOpening40ValidateSource(tx, cfg, progress.PlayerID); err != nil {
		return err
	}
	var old db.PeriodProgress
	err := tx.Where("instance_id=? AND player_id=?", cfg.RewardSourceID, progress.PlayerID).Take(&old).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if old.Value < 0 {
		return constUtil.ErrAdminConfiguration
	}
	progress.Value, progress.LastDay = old.Value, old.LastDay
	return nil
}
func personalOpening40RenewDelivery(tx *gorm.DB, playerID int64, cfg db.PeriodInstance, delivery *db.RewardDelivery, now int64) error {
	if err := personalOpening40ValidateSource(tx, cfg, playerID); err != nil {
		return err
	}
	if delivery.ClaimedAt != 0 || now < cfg.StartAt || now >= cfg.RewardEndAt || delivery.ExpiresAt == 0 || delivery.ExpiresAt >= cfg.RewardEndAt {
		return nil
	}
	if err := tx.Model(&db.RewardDelivery{}).Where("id=? AND player_id=? AND claimed_at=0 AND expires_at>0 AND expires_at<?", delivery.ID, playerID, cfg.RewardEndAt).Update("expires_at", cfg.RewardEndAt).Error; err != nil {
		return err
	}
	delivery.ExpiresAt = cfg.RewardEndAt
	return nil
}
func personalOpening40TabScope(cfg db.PeriodInstance) string {
	if cfg.RewardSourceID != "" || strings.Contains(cfg.ID, "_character_") {
		return constUtil.PersonalOpening40TabScope
	}
	return constUtil.PersonalOpening40CalendarScope
}
func personalOpening40DTO(row db.AdminActivityOccurrence) (model.AdminPersonalOpening40DTO, error) {
	var form model.AdminActivityForm
	if json.Unmarshal(row.FormJSON, &form) != nil {
		return model.AdminPersonalOpening40DTO{}, constUtil.ErrAdminConfiguration
	}
	form = adminActivityCanonicalForm(form)
	return model.AdminPersonalOpening40DTO{ID: row.ID, Name: form.Name, Mode: row.Mode, StartDay: row.StartDay, DurationDays: row.DurationDays, ClaimDays: row.ClaimDays, Enabled: row.Enabled && !row.Archived, Version: adminVersion(row), Form: form}, nil
}

// AdminPersonalOpening40 lists only the selected realm's stable personal rules.
func (s *GameService) AdminPersonalOpening40(ctx context.Context, p *model.AdminPrincipal, server int64) ([]model.AdminPersonalOpening40DTO, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	if err := adminRequireActivityServer(tx, server); err != nil {
		return nil, err
	}
	var rows []db.AdminActivityOccurrence
	if err := tx.Where("server_id=? AND timing=? AND archived=?", server, "character-day", false).Order("mode,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []model.AdminPersonalOpening40DTO{}
	for _, row := range rows {
		definition, supported := adminActivityDefinitionFor(row.Mode)
		if personalOpening40Rule(row) || supported && definition.domain == "period" {
			dto, err := personalOpening40DTO(row)
			if err != nil {
				return nil, err
			}
			out = append(out, dto)
		}
	}
	return out, nil
}

// AdminSavePersonalOpening40 updates the persistent source for future roles only.
func (s *GameService) AdminSavePersonalOpening40(ctx context.Context, p *model.AdminPrincipal, server int64, id string, req model.AdminPersonalOpening40SaveRequest) (model.AdminPersonalOpening40DTO, error) {
	var out model.AdminPersonalOpening40DTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := adminRequireActivityServer(tx, server); err != nil {
			return err
		}
		revision, err := adminActivityScheduleRevision(tx, server, true)
		if err != nil {
			return err
		}
		var row db.AdminActivityOccurrence
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND server_id=?", id, server).Error; err != nil {
			return err
		}
		definitionCheck, supported := adminActivityDefinitionFor(row.Mode)
		if row.Timing != "character-day" || !supported || definitionCheck.domain != "period" || row.Archived || adminVersion(row) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		var form model.AdminActivityForm
		if json.Unmarshal(row.FormJSON, &form) != nil {
			return constUtil.ErrAdminConfiguration
		}
		// Only the selected rule's editable thresholds and resources are accepted.
		form.Milestones = req.Form.Milestones
		form.StartAt = time.Now().UnixMilli()
		form.EndAt = form.StartAt + req.DurationDays*86400000
		form.ClaimEndAt = form.EndAt + req.ClaimDays*86400000
		definition, ok := adminActivityDefinitionFor(row.Mode)
		if !ok {
			return constUtil.ErrAdminConfiguration
		}
		if err = tx.SavePoint("personal40_validate").Error; err != nil {
			return err
		}
		validation := s.adminPersistTypedActivity(tx, definition, row.ConsumerKey+"_check", form, false, false)
		if err = tx.RollbackTo("personal40_validate").Error; err != nil {
			return err
		}
		if validation != nil {
			return validation
		}
		// Existing instances keep their FormJSON and absolute dates; the source switch remains authoritative.
		row.FormJSON = adminJSON(form)
		row.StartDay = req.StartDay
		row.DurationDays = req.DurationDays
		row.ClaimDays = req.ClaimDays
		row.Enabled = req.Enabled
		row.Revision++
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
		revision.Revision++
		if err = tx.Save(&revision).Error; err != nil {
			return err
		}
		if err = adminAudit(tx, p, constUtil.PersonalOpening40SaveAction, id, "更新未来个人活动规则；已生成实例与历史领取保持"); err != nil {
			return err
		}
		out, err = personalOpening40DTO(row)
		return err
	})
	return out, err
}

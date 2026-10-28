package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// feedback79WheelForm preserves unrecognized JSON fields while changing only
// the two wheel settings. Issued character snapshots are never rewritten.
func feedback79WheelForm(raw []byte) ([]byte, bool, error) {
	var form model.AdminActivityForm
	if err := json.Unmarshal(raw, &form); err != nil {
		return nil, false, constUtil.ErrAdminConfiguration
	}
	if form.Mode != "turntable" || form.Source != constUtil.Feedback79GrowthSource || form.TemplateID != constUtil.Feedback79TurntableTemplate {
		return raw, false, nil
	}
	if form.Options.InitialDiamondPool != 0 || !model.TurntablePrizePityValid(form.Options.PrizePityRules, constUtil.Feedback79JackpotPerMillion) {
		return nil, false, fmt.Errorf("%w: 转盘首池或保底配置不符", constUtil.ErrAdminConfigConflict)
	}
	jackpot, contribution := form.Options.JackpotPerMillion, form.Options.PoolContributionBps
	if jackpot == constUtil.Feedback79JackpotPerMillion && contribution == constUtil.Feedback79ContributionBps {
		return raw, true, nil
	}
	if jackpot != 0 || contribution != 0 {
		return nil, false, fmt.Errorf("%w: 转盘概率或返池已被修改", constUtil.ErrAdminConfigConflict)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, false, err
	}
	var options map[string]json.RawMessage
	if err := json.Unmarshal(object["options"], &options); err != nil || options == nil {
		return nil, false, constUtil.ErrAdminConfiguration
	}
	options["jackpotPerMillion"] = json.RawMessage("200")
	options["poolContributionBps"] = json.RawMessage("200")
	updatedOptions, err := json.Marshal(options)
	if err != nil {
		return nil, false, err
	}
	object["options"] = updatedOptions
	updated, err := json.Marshal(object)
	return updated, true, err
}

// ApplyFeedback79Config changes only the reviewed V12 wheel settings, then
// applies the membership curve. Both steps require maintenance and are retryable.
func (s *GameService) ApplyFeedback79Config(ctx context.Context, actor *model.AdminPrincipal) error {
	serverID := s.OriginalServerID()
	if serverID != 1 || !s.ServerClockReady {
		return constUtil.ErrAdminConfiguration
	}
	ctx, _, err := s.bindServerGameClockWorker75(ctx)
	if err != nil {
		return err
	}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var clock db.ServerGameClock
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&clock, "server_id=?", serverID).Error; err != nil {
			return err
		}
		if !serverGameClockValid(&clock) {
			return constUtil.ErrAdminConfiguration
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var user db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, actor.UserID).Error; err != nil {
			return err
		}
		if !user.Enabled || user.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		var maintenance db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&maintenance, 1).Error; err != nil {
			return err
		}
		if !maintenance.Maintenance {
			return fmt.Errorf("%w: 须在维护期执行", constUtil.ErrAdminConfigConflict)
		}
		var revision db.AdminActivityScheduleRevision
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&revision, "server_id=?", serverID).Error; err != nil {
			return err
		}
		var applied int64
		if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.Feedback79AuditAction, fmt.Sprint(serverID)).Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil
		}
		var schedules []db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND mode=? AND timing=? AND archived=?", serverID, "turntable", "character-day", false).Order("id").Limit(constUtil.Feedback79MaxSchedules + 1).Find(&schedules).Error; err != nil {
			return err
		}
		if len(schedules) > constUtil.Feedback79MaxSchedules {
			return fmt.Errorf("%w: 角色转盘排期超出上限", constUtil.ErrAdminConfigConflict)
		}
		var occurrenceIDs, templateIDs []string
		seenTemplates := map[string]bool{}
		for _, row := range schedules {
			raw, match, err := feedback79WheelForm(row.FormJSON)
			if err != nil {
				return err
			}
			if !match {
				continue
			}
			if row.ConsumerKey == "" || row.ConsumerID != "character-plan:"+row.ConsumerKey || row.TemplateID == "" {
				return constUtil.ErrAdminConfigConflict
			}
			occurrenceIDs = append(occurrenceIDs, row.ID)
			if !seenTemplates[row.TemplateID] {
				seenTemplates[row.TemplateID] = true
				templateIDs = append(templateIDs, row.TemplateID)
			}
			if string(raw) != string(row.FormJSON) {
				if err := tx.Model(&db.AdminActivityOccurrence{}).Where("id=?", row.ID).Updates(map[string]any{"form_json": raw, "revision": row.Revision + 1}).Error; err != nil {
					return err
				}
			}
		}
		if len(occurrenceIDs) == 0 {
			return fmt.Errorf("%w: 找不到成长V12转盘排期", constUtil.ErrAdminConfigConflict)
		}
		var templates []db.AdminActivityDefinition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", templateIDs).Order("id").Find(&templates).Error; err != nil {
			return err
		}
		if len(templates) != len(templateIDs) {
			return fmt.Errorf("%w: 转盘模板缺失", constUtil.ErrAdminConfigConflict)
		}
		for _, row := range templates {
			raw, match, err := feedback79WheelForm(row.FormJSON)
			if err != nil || !match || row.Mode != "turntable" {
				return fmt.Errorf("%w: 转盘模板与排期不符 %s", constUtil.ErrAdminConfigConflict, row.ID)
			}
			if string(raw) != string(row.FormJSON) {
				if err := tx.Model(&db.AdminActivityDefinition{}).Where("id=?", row.ID).Updates(map[string]any{"form_json": raw, "revision": row.Revision + 1, "updated_at_millis": max(time.Now().UnixMilli(), row.UpdatedAtMillis+1)}).Error; err != nil {
					return err
				}
			}
		}
		now := serverGameClockView(&clock, time.Now().UTC()).GameTimeMS
		var issued []db.AdminCharacterActivityInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND mode=? AND occurrence_id IN ? AND start_at<=? AND end_at>?", serverID, "turntable", occurrenceIDs, now, now).Order("consumer_key").Limit(constUtil.Feedback79MaxActiveInstances + 1).Find(&issued).Error; err != nil {
			return err
		}
		if len(issued) > constUtil.Feedback79MaxActiveInstances {
			return fmt.Errorf("%w: 当前转盘实例超出上限", constUtil.ErrAdminConfigConflict)
		}
		keys := make([]string, 0, len(issued))
		issuedByKey := make(map[string]db.AdminCharacterActivityInstance, len(issued))
		for _, instance := range issued {
			_, match, err := feedback79WheelForm(instance.FormJSON)
			digest := p6Digest([]byte(fmt.Sprintf("%d/%d/%s", serverID, instance.PlayerID, instance.OccurrenceID)))[:20]
			key := "admin_turntable_character_" + digest
			if err != nil || !match || instance.PlayerID <= 0 || instance.ConsumerKey != key || instance.ConsumerID != "pool:"+key {
				return fmt.Errorf("%w: 当前转盘实例身份不符", constUtil.ErrAdminConfigConflict)
			}
			keys = append(keys, key)
			issuedByKey[key] = instance
		}
		var changed int
		if len(keys) > 0 {
			var pools []db.GameplayPool
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", keys).Order("id").Find(&pools).Error; err != nil {
				return err
			}
			if len(pools) != len(keys) {
				return fmt.Errorf("%w: 当前转盘池缺失", constUtil.ErrAdminConfigConflict)
			}
			var rules []db.TurntableRules
			if err := tx.Where("pool_id IN ?", keys).Find(&rules).Error; err != nil {
				return err
			}
			validRules := map[string]bool{}
			for _, rule := range rules {
				if _, err := turntablePityRules(rule.PrizePityRules, int32(constUtil.Feedback79JackpotPerMillion)); err != nil {
					return fmt.Errorf("%w: 转盘保底与大奖概率冲突 %s", constUtil.ErrAdminConfigConflict, rule.PoolID)
				}
				validRules[rule.PoolID] = true
			}
			var pending []string
			for _, pool := range pools {
				instance := issuedByKey[pool.ID]
				if pool.Source != constUtil.Feedback79GrowthSource || pool.InitialDiamondPool != 0 || !validRules[pool.ID] || pool.StartAt != instance.StartAt || pool.EndAt != instance.EndAt || pool.RewardEndAt != instance.ClaimEndAt {
					return fmt.Errorf("%w: 当前转盘池来源或首池不符 %s", constUtil.ErrAdminConfigConflict, pool.ID)
				}
				if pool.JackpotPerMillion == int32(constUtil.Feedback79JackpotPerMillion) && pool.PoolContributionBps == int32(constUtil.Feedback79ContributionBps) {
					continue
				}
				if pool.JackpotPerMillion != 0 || pool.PoolContributionBps != 0 {
					return fmt.Errorf("%w: 当前转盘池概率或返池已被修改 %s", constUtil.ErrAdminConfigConflict, pool.ID)
				}
				pending = append(pending, pool.ID)
			}
			if len(pending) > 0 {
				result := tx.Model(&db.GameplayPool{}).Where("id IN ? AND jackpot_per_million=? AND pool_contribution_bps=?", pending, 0, 0).Updates(map[string]any{"jackpot_per_million": constUtil.Feedback79JackpotPerMillion, "pool_contribution_bps": constUtil.Feedback79ContributionBps})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != int64(len(pending)) {
					return constUtil.ErrAdminConfigConflict
				}
				changed = len(pending)
			}
		}
		if err := tx.Model(&db.AdminActivityScheduleRevision{}).Where("server_id=?", serverID).Update("revision", revision.Revision+1).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, constUtil.Feedback79AuditAction, fmt.Sprint(serverID), fmt.Sprintf("成长V12转盘%d排期、%d模板、%d当前池：大奖200ppm、付费返池2%%；首池和历史保持", len(occurrenceIDs), len(templateIDs), changed))
	})
	if err != nil {
		return err
	}
	return s.ApplyActivity77Member(ctx, actor)
}

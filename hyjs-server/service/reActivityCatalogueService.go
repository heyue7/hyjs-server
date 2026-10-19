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

// adminReActivityVerifiedSource identifies confirmed Re restoration and the user's original screenshot pool.
// Explicit operator edits are checked separately using exact audited identities.
func adminReActivityVerifiedSource(id string, form model.AdminActivityForm) bool {
	return id == "pool:"+constUtil.TurntableScreenshotPool || id == "pool:recruit_3" || form.Mode == "shop-window" || strings.Contains(form.Source, "Re v1.0.5.1")
}
func adminActivityOperatorReviewed(tx *gorm.DB, id, sourceID string) (bool, error) {
	var count int64
	err := tx.Model(&db.AdminAudit{}).Where("actor<>? AND ((target=? AND action IN ?) OR (target=? AND action IN ?))", "system", id, []string{"activity.template.save", "activity.template.archive"}, sourceID, []string{"activity.create", "activity.update", "activity.state"}).Count(&count).Error
	return count > 0, err
}
func adminActivityAutomaticImport(row db.AdminActivityDefinition) bool {
	return row.SourceActivityID != "" && row.ID == "import_"+p6Digest([]byte(row.SourceActivityID))[:24]
}
func adminReActivityCatalogueStarted(tx *gorm.DB) (bool, error) {
	if !tx.Migrator().HasTable(&db.AdminAudit{}) {
		return false, nil
	}
	var n int64
	err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.ReActivityCatalogueCleanupAction, constUtil.ReActivityCatalogueCleanupTarget).Count(&n).Error
	return n > 0, err
}

// adminActivityHideUnverifiedSource prevents monthly legacy placeholders from returning after cleanup.
func adminActivityHideUnverifiedSource(tx *gorm.DB, id string, form model.AdminActivityForm) (bool, error) {
	started, err := adminReActivityCatalogueStarted(tx)
	if err != nil || !started || adminReActivityVerifiedSource(id, form) {
		return false, err
	}
	if strings.Contains(id, "_schedule_") || strings.Contains(id, "_character_") {
		return false, nil
	}
	reviewed, err := adminActivityOperatorReviewed(tx, "import_"+p6Digest([]byte(id))[:24], id)
	return !reviewed, err
}

// InitializeReActivityCatalogue archives unverified operational placeholders and issues confirmed Re configurations once.
// It runs after calendar adoption and Re restaurant restoration; no player progress, stock, receipt or accepted TTL is deleted.
func (s *GameService) InitializeReActivityCatalogue(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		serverID := s.OriginalServerID()
		revision, err := adminActivityScheduleRevision(tx, serverID, true)
		if err != nil {
			return err
		}
		target := constUtil.ReActivityCatalogueCleanupTarget + ":" + strconv.FormatInt(serverID, 10)
		var applied int64
		if err = tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.ReActivityCatalogueCleanupAction, target).Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil
		}
		var definitions []db.AdminActivityDefinition
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("archived=?", false).Order("id").Find(&definitions).Error; err != nil {
			return err
		}
		archived := []string{}
		for _, row := range definitions {
			// Preserve the original opening task program explicitly retained by the user.
			if row.Mode == "opening-14" {
				continue
			}
			var form model.AdminActivityForm
			if json.Unmarshal(row.FormJSON, &form) != nil {
				return constUtil.ErrAdminConfiguration
			}
			if adminReActivityVerifiedSource(row.SourceActivityID, form) {
				continue
			}
			row.Archived = true
			row.Enabled = false
			row.UnavailableReason = constUtil.ReActivityCatalogueMissingSource
			row.Revision++
			row.UpdatedAtMillis = time.Now().UnixMilli()
			if err = tx.Save(&row).Error; err != nil {
				return err
			}
			archived = append(archived, row.ID)
			// Ordinary shops and basic recruits keep their own menu and consumer; only their redundant activity aliases disappear.
			if row.SourceActivityID != "" && !adminActivityRedundantSource(row.SourceActivityID) {
				if err = adminEnableActivityConsumer(tx, row.SourceActivityID, false); err != nil {
					return err
				}
				meta := db.AdminActivityPresentation{ActivityID: row.SourceActivityID, Name: row.Name, Mode: row.Mode, TemplateID: form.TemplateID, Archived: true, UpdatedAtMillis: time.Now().UnixMilli()}
				if err = tx.Save(&meta).Error; err != nil {
					return err
				}
			}
		}
		if len(archived) > 0 {
			var occurrences []db.AdminActivityOccurrence
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND template_id IN ? AND archived=?", serverID, archived, false).Order("id").Find(&occurrences).Error; err != nil {
				return err
			}
			for _, row := range occurrences {
				if adminActivityTiming(row) != "character-day" && row.Mode != "opening-14" {
					if err = adminEnableActivityConsumer(tx, row.ConsumerID, false); err != nil {
						return err
					}
				}
				if err = tx.Model(&row).Updates(map[string]any{"archived": true, "enabled": false, "revision": row.Revision + 1}).Error; err != nil {
					return err
				}
			}
		}
		// These two legacy catalogue domains are absent from the typed template list.
		if tx.Migrator().HasTable(&db.ActivityRules{}) {
			if err = tx.Model(&db.ActivityRules{}).Where("id=?", constUtil.P1LegacyLoginID).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable(&db.P15SeasonalBundle{}) {
			if err = tx.Model(&db.P15SeasonalBundle{}).Where("enabled=?", true).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		// Stop legacy payment placeholders without deleting their order, claim or membership history.
		if tx.Migrator().HasTable(&db.PaymentActivityRule{}) {
			if err = tx.Model(&db.PaymentActivityRule{}).Where("kind IN ?", []string{"single", "total", "continued", "consume"}).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		now := LocalPlayNow(tx.Statement.Context)
		// Startup has no bound request clock; read the saved realm offset under the existing clock guard.
		if tx.Migrator().HasTable(&db.ServerGameClock{}) {
			var clock db.ServerGameClock
			e := tx.First(&clock, "server_id=?", s.serverGameClockID()).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if e == nil {
				now = time.Now().AddDate(0, 0, int(clock.OffsetDays))
			}
		}
		local := now.In(time.FixedZone("UTC+8", 8*3600))
		day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		back := (int(day.Weekday()) + 6) % 7
		start := day.AddDate(0, 0, -back)
		end := start.AddDate(0, 0, 7)
		for _, form := range append(s.reActivityForms(), reOperationsWelfareForms()...) {
			if err = s.adminRestoreReActivity(tx, serverID, form, start, end, now); err != nil {
				return err
			}
		}
		revision.Revision++
		if err = tx.Save(&revision).Error; err != nil {
			return err
		}
		actor := &model.AdminPrincipal{Username: "system"}
		if err = adminAudit(tx, actor, constUtil.ReActivityCatalogueCleanupAction, target, fmt.Sprintf("归档%d个无Re投放依据的旧模板；真实Re每类完整档位当前周独立发布，历史保持", len(archived))); err != nil {
			return err
		}
		// The global marker only governs future automatically generated catalogue aliases.
		global, e := adminReActivityCatalogueStarted(tx)
		if e != nil {
			return e
		}
		if !global {
			return adminAudit(tx, actor, constUtil.ReActivityCatalogueCleanupAction, constUtil.ReActivityCatalogueCleanupTarget, "隐藏后续旧占位导入，手配活动继续可用")
		}
		return nil
	})
}
func (s *GameService) reActivityForms() []model.AdminActivityForm {
	out := []model.AdminActivityForm{}
	for _, spec := range []struct {
		mode, name string
		tiers      []p1PeriodMilestone
	}{
		{"login-total", "Re累计登录", []p1PeriodMilestone{{Goal: 1, Rewards: "coin,100000"}, {Goal: 2, Rewards: "item_007,40"}, {Goal: 3, Rewards: "item_010,30"}, {Goal: 4, Rewards: "item_006,60"}, {Goal: 5, Rewards: "item_005,60"}, {Goal: 6, Rewards: "diamond,300"}, {Goal: 7, Rewards: "item_016,5"}}},
		{"single-spend", "Re消费奖励", []p1PeriodMilestone{{Goal: 600, Rewards: "item_002,10"}, {Goal: 3000, Rewards: "item_006,50"}, {Goal: 9800, Rewards: "item_010,30"}, {Goal: 19800, Rewards: "diamond,300"}, {Goal: 32800, Rewards: "diamond,800"}, {Goal: 64800, Rewards: "diamond,2000"}}},
		{"arena-wins", "Re竞技胜场", []p1PeriodMilestone{{Goal: 1, Rewards: "item_002,10"}, {Goal: 3, Rewards: "coin,200000"}, {Goal: 5, Rewards: "item_006,50"}, {Goal: 10, Rewards: "item_010,30"}, {Goal: 20, Rewards: "diamond,300"}, {Goal: 50, Rewards: "diamond,800"}}},
		{"plunder-count", "Re夺宝次数", []p1PeriodMilestone{{Goal: 1, Rewards: "item_002,10"}, {Goal: 3, Rewards: "coin,200000"}, {Goal: 5, Rewards: "item_006,50"}, {Goal: 10, Rewards: "item_010,30"}, {Goal: 20, Rewards: "diamond,300"}, {Goal: 50, Rewards: "diamond,800"}}},
		{"wanted-attempts", "Re悬赏挑战", []p1PeriodMilestone{{Goal: 1, Rewards: "item_002,10"}, {Goal: 3, Rewards: "coin,200000"}, {Goal: 5, Rewards: "item_006,50"}, {Goal: 10, Rewards: "item_010,30"}, {Goal: 20, Rewards: "diamond,300"}, {Goal: 50, Rewards: "diamond,800"}}},
		{"wanted-kills", "Re悬赏击杀", []p1PeriodMilestone{{Goal: 1, Rewards: "item_002,10"}, {Goal: 3, Rewards: "coin,200000"}, {Goal: 5, Rewards: "item_006,50"}, {Goal: 10, Rewards: "item_010,30"}, {Goal: 20, Rewards: "diamond,300"}, {Goal: 50, Rewards: "diamond,800"}}},
		{"copy-win", "Re主线通关", []p1PeriodMilestone{{Goal: 1, Rewards: "item_002,10"}, {Goal: 3, Rewards: "coin,200000"}, {Goal: 5, Rewards: "item_006,50"}, {Goal: 10, Rewards: "item_010,30"}, {Goal: 20, Rewards: "diamond,300"}, {Goal: 50, Rewards: "diamond,800"}}},
		{"diamond-spent", "Re累计消费", []p1PeriodMilestone{{Goal: 1000, Rewards: "coin,100000"}, {Goal: 5000, Rewards: "item_002,10"}, {Goal: 10000, Rewards: "item_006,50"}, {Goal: 50000, Rewards: "item_010,30"}, {Goal: 100000, Rewards: "diamond,500"}}},
	} {
		form := model.AdminActivityForm{Mode: spec.mode, Name: spec.name, TemplateID: spec.mode + "-original", Source: constUtil.ReActivityCatalogueSource, RepeatPolicy: "repeat", Products: []model.AdminActivityProduct{}, Prizes: []model.AdminActivityPrize{}, Milestones: []model.AdminActivityMilestone{}}
		for _, tier := range spec.tiers {
			form.Milestones = append(form.Milestones, model.AdminActivityMilestone{Goal: tier.Goal, Rewards: adminTypedResources(tier.Rewards)})
		}
		out = append(out, form)
	}
	sign := model.AdminActivityForm{Mode: "limited-sign", Name: "Re限时签到", TemplateID: "limited-sign-original", Source: constUtil.ReActivityCatalogueSource, RepeatPolicy: "repeat", Products: []model.AdminActivityProduct{}, Prizes: []model.AdminActivityPrize{}, Milestones: []model.AdminActivityMilestone{}}
	for day := int32(1); day <= 7; day++ {
		sign.Milestones = append(sign.Milestones, model.AdminActivityMilestone{Goal: int64(day), Rewards: adminTypedResources(reMonthSignReward(day))})
	}
	out = append(out, sign)
	work := model.AdminActivityForm{Mode: "work", Name: "Re开工挑战", TemplateID: "work-original", Source: constUtil.ReActivityCatalogueSource + " 原开服表前12条非空奖励，真实周主线通关1..12。", RepeatPolicy: "repeat", Products: []model.AdminActivityProduct{}, Prizes: []model.AdminActivityPrize{}, Milestones: []model.AdminActivityMilestone{}}
	for _, row := range s.Tables["ActivityNewOpen7Day"] {
		reward := strings.TrimSuffix(strings.TrimSpace(row["reward"]), ";")
		if reward == "" {
			continue
		}
		work.Milestones = append(work.Milestones, model.AdminActivityMilestone{Goal: int64(len(work.Milestones) + 1), Rewards: adminTypedResources(reward)})
		if len(work.Milestones) == 12 {
			break
		}
	}
	if len(work.Milestones) == 12 {
		out = append(out, work)
	}
	return out
}
func (s *GameService) adminRestoreReActivity(tx *gorm.DB, serverID int64, form model.AdminActivityForm, start, end, now time.Time) error {
	d, ok := adminActivityDefinitionFor(form.Mode)
	if !ok {
		return constUtil.ErrAdminConfiguration
	}
	key := d.prefix + "re1051_schedule_" + strconv.FormatInt(serverID, 10)
	sourceID := d.domain + ":" + key
	definitionID := "re1051_" + strings.ReplaceAll(form.Mode, "-", "_")
	var definition db.AdminActivityDefinition
	err := tx.First(&definition, "id=?", definitionID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		definition = db.AdminActivityDefinition{ID: definitionID, Name: form.Name, Mode: form.Mode, FormJSON: adminJSON(form), DefaultDays: 7, Usable: true, Enabled: true, SourceActivityID: sourceID, Revision: 1, UpdatedAtMillis: time.Now().UnixMilli()}
		if err = tx.Create(&definition).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if definition.Archived || !definition.Enabled {
		return nil
	}
	id := "restore_re1051_" + strings.ReplaceAll(form.Mode, "-", "_") + "_" + strconv.FormatInt(serverID, 10)
	var exists int64
	if err = tx.Model(&db.AdminActivityOccurrence{}).Where("id=?", id).Count(&exists).Error; err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	// An operator's valid live activity of this kind remains the one visible instance.
	var active int64
	if err = tx.Model(&db.AdminActivityOccurrence{}).Where("server_id=? AND mode=? AND enabled=? AND archived=? AND start_at<=? AND claim_end_at>?", serverID, form.Mode, true, false, now.UnixMilli(), now.UnixMilli()).Count(&active).Error; err != nil {
		return err
	}
	if active > 0 {
		return nil
	}
	if json.Unmarshal(definition.FormJSON, &form) != nil {
		return constUtil.ErrAdminConfiguration
	}
	form.StartAt = start.UnixMilli()
	form.EndAt = end.UnixMilli()
	form.ClaimEndAt = end.UnixMilli()
	if err = s.adminPersistTypedActivity(tx, d, key, form, true, false); err != nil {
		return err
	}
	meta := db.AdminActivityPresentation{ActivityID: sourceID, Mode: form.Mode, TemplateID: form.TemplateID, Name: form.Name, UpdatedAtMillis: time.Now().UnixMilli()}
	if err = tx.Create(&meta).Error; err != nil {
		return err
	}
	row := db.AdminActivityOccurrence{ID: id, ServerID: serverID, TemplateID: definition.ID, TemplateVersion: adminVersion(definition), Name: form.Name, Mode: form.Mode, Timing: "calendar", ConsumerKey: key, ConsumerID: sourceID, StartAt: form.StartAt, EndAt: form.EndAt, ClaimEndAt: form.ClaimEndAt, Lane: 2, Enabled: true, FormJSON: adminJSON(form), Revision: 1}
	return tx.Create(&row).Error
}

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func feedback78FormValid(raw []byte) bool {
	var form model.AdminActivityForm
	if json.Unmarshal(raw, &form) != nil || form.Name != constUtil.Feedback78ActivityName ||
		form.Mode != constUtil.Feedback78ActivityMode || form.TemplateID != "login-total-original" ||
		form.Source != constUtil.Feedback78ActivitySource || len(form.Milestones) != 1 {
		return false
	}
	tier := form.Milestones[0]
	return tier.Goal == 3 && len(tier.Options) == 0 && len(tier.Rewards) == 1 &&
		tier.Rewards[0].ResourceID == "item_9711" && tier.Rewards[0].Amount == 288
}

func feedback78PeriodValid(cfg db.PeriodInstance, id string) bool {
	if cfg.ID != id || cfg.Source != constUtil.Feedback78ActivitySource ||
		cfg.Kind != constUtil.P1EventLoginDay || cfg.Mode != "cumulative" || cfg.Goal != 3 ||
		cfg.Rewards != constUtil.Feedback78ActivityReward || cfg.RewardSourceID != "" || cfg.RankPlaces != 0 {
		return false
	}
	tiers := p1PeriodMilestones(cfg)
	return len(tiers) == 1 && tiers[0].Goal == 3 && tiers[0].Rewards == constUtil.Feedback78ActivityReward &&
		tiers[0].SourceKey == "" && len(p1PeriodSources(cfg)) == 1 && p1PeriodSources(cfg)[0] == "activity:"+id
}

// feedback78Preflight caps both the primary-key receipt scan and matching progress snapshots.
// OFFSET probes inspect only indexed identifiers and refuse before changing any rows.
func feedback78Preflight(tx *gorm.DB, periods []string, maximumDeliveries, maximumProgress int) (int64, error) {
	var latest []db.RewardDelivery
	if err := tx.Select("id").Order("id DESC").Limit(1).Find(&latest).Error; err != nil {
		return 0, err
	}
	var ceiling int64
	if len(latest) > 0 {
		ceiling = latest[0].ID
	}
	var delivery []db.RewardDelivery
	if err := tx.Select("id").Where("id<=?", ceiling).Order("id ASC").Offset(maximumDeliveries).Limit(1).Find(&delivery).Error; err != nil {
		return 0, err
	}
	if len(delivery) > 0 {
		return 0, fmt.Errorf("%w: 奖励记录超过维护扫描上限", constUtil.ErrAdminConfigConflict)
	}
	var progress []db.PeriodProgress
	if err := tx.Select("instance_id", "player_id").Where("instance_id IN ?", periods).
		Order("instance_id ASC,player_id ASC").Offset(maximumProgress).Limit(1).Find(&progress).Error; err != nil {
		return 0, err
	}
	if len(progress) > 0 {
		return 0, fmt.Errorf("%w: 活动快照超过维护扫描上限", constUtil.ErrAdminConfigConflict)
	}
	return ceiling, nil
}

// feedback78ValidateProgress checks every existing snapshot in bounded primary-key pages.
// The reward source may only be retired after all historical snapshots agree with its period.
func feedback78ValidateProgress(tx *gorm.DB, cfg db.PeriodInstance) error {
	var last int64
	for {
		var rows []db.PeriodProgress
		if err := tx.Select("instance_id", "player_id", "snapshot").Where("instance_id=? AND player_id>?", cfg.ID, last).
			Order("player_id ASC").Limit(constUtil.Feedback78BatchSize).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var snapshot db.PeriodInstance
			if row.PlayerID <= last || json.Unmarshal(row.Snapshot, &snapshot) != nil || !feedback78PeriodValid(snapshot, cfg.ID) {
				return fmt.Errorf("%w: 活动进度快照或奖励来源不符 %s/%d", constUtil.ErrAdminConfigConflict, cfg.ID, row.PlayerID)
			}
			last = row.PlayerID
		}
		if len(rows) < constUtil.Feedback78BatchSize {
			return nil
		}
	}
}

// feedback78RetireDeliveries also covers orphaned receipts without a progress row.
// The schema has no source-only index, so one primary-key scan uses bounded memory and exact source keys.
func feedback78RetireDeliveries(tx *gorm.DB, sources []string, now, ceiling int64) (int64, error) {
	var last, changed int64
	for {
		var rows []db.RewardDelivery
		if err := tx.Select("id").Where("id>? AND id<=? AND source_key IN ? AND claimed_at=0 AND (expires_at=0 OR expires_at>?)", last, ceiling, sources, now).
			Order("id ASC").Limit(constUtil.Feedback78BatchSize).Find(&rows).Error; err != nil {
			return changed, err
		}
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
			last = row.ID
		}
		if len(ids) > 0 {
			result := tx.Model(&db.RewardDelivery{}).Where("id IN ? AND source_key IN ? AND claimed_at=0 AND (expires_at=0 OR expires_at>?)", ids, sources, now).
				Update("expires_at", now)
			if result.Error != nil {
				return changed, result.Error
			}
			changed += result.RowsAffected
		}
		if len(rows) < constUtil.Feedback78BatchSize {
			return changed, nil
		}
	}
}

// ApplyFeedback78Config atomically retires the verified unwanted activity and reprices the existing VIP10 product.
// It is an explicit maintenance operation; it preserves frozen rules, progress, claimed receipts and player assets.
func (s *GameService) ApplyFeedback78Config(ctx context.Context, actor *model.AdminPrincipal) error {
	serverID := s.OriginalServerID()
	if serverID != 1 {
		return constUtil.ErrAdminConfiguration
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		realNow := time.Now().UTC()
		now := realNow.UnixMilli()
		// Use the same realm clock as reward claims and hold it stable through commit.
		if s.ServerClockReady {
			var clock db.ServerGameClock
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&clock, "server_id=?", serverID).Error; err != nil {
				return err
			}
			if !serverGameClockValid(&clock) {
				return constUtil.ErrAdminConfiguration
			}
			now = serverGameClockView(&clock, realNow).GameTimeMS
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
		var policy db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Maintenance {
			return fmt.Errorf("%w: 须在维护期执行", constUtil.ErrAdminConfigConflict)
		}
		var revision db.AdminActivityScheduleRevision
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&revision, "server_id=?", serverID).Error; err != nil {
			return err
		}
		var initial []db.AdminActivityOccurrence
		if err := tx.Where("server_id=? AND template_id=?", serverID, constUtil.Feedback78TemplateID).
			Order("id ASC").Limit(constUtil.Feedback78ScheduleCount + 1).Find(&initial).Error; err != nil {
			return err
		}
		if len(initial) != constUtil.Feedback78ScheduleCount {
			return fmt.Errorf("%w: 目标排期数量不符", constUtil.ErrAdminConfigConflict)
		}
		periodIDs := make([]string, 0, len(initial))
		seen := map[string]bool{}
		for _, row := range initial {
			if row.ConsumerKey == "" || row.ConsumerID != "period:"+row.ConsumerKey || seen[row.ConsumerKey] {
				return fmt.Errorf("%w: 排期消费者身份不符", constUtil.ErrAdminConfigConflict)
			}
			seen[row.ConsumerKey] = true
			periodIDs = append(periodIDs, row.ConsumerKey)
		}
		sort.Strings(periodIDs)
		var periods []db.PeriodInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", periodIDs).Order("id ASC").Find(&periods).Error; err != nil {
			return err
		}
		if len(periods) != len(periodIDs) {
			return fmt.Errorf("%w: 目标活动实例缺失", constUtil.ErrAdminConfigConflict)
		}
		ceiling, err := feedback78Preflight(tx, periodIDs, constUtil.Feedback78MaxDeliveryScan, constUtil.Feedback78MaxProgressRows)
		if err != nil {
			return err
		}
		byID := make(map[string]db.PeriodInstance, len(periods))
		for _, period := range periods {
			if period.Title != constUtil.Feedback78ActivityName || !feedback78PeriodValid(period, period.ID) {
				return fmt.Errorf("%w: 活动实例规则不符 %s", constUtil.ErrAdminConfigConflict, period.ID)
			}
			frozen, err := p1PeriodSnapshot(period)
			if err != nil || !feedback78PeriodValid(frozen, period.ID) {
				return fmt.Errorf("%w: 活动冻结规则不符 %s", constUtil.ErrAdminConfigConflict, period.ID)
			}
			if err = feedback78ValidateProgress(tx, period); err != nil {
				return err
			}
			byID[period.ID] = period
		}
		var occurrences []db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND template_id=?", serverID, constUtil.Feedback78TemplateID).
			Order("start_at ASC,id ASC").Limit(constUtil.Feedback78ScheduleCount + 1).Find(&occurrences).Error; err != nil {
			return err
		}
		if len(occurrences) != constUtil.Feedback78ScheduleCount {
			return fmt.Errorf("%w: 目标排期已变化", constUtil.ErrAdminConfigConflict)
		}
		for i, row := range occurrences {
			period, ok := byID[row.ConsumerKey]
			if !ok || row.ConsumerID != "period:"+row.ConsumerKey || row.Name != constUtil.Feedback78ActivityName ||
				row.Mode != constUtil.Feedback78ActivityMode || adminActivityTiming(row) != "calendar" || row.Lane != 6 ||
				row.EndAt-row.StartAt != int64(7*24*time.Hour/time.Millisecond) || row.ClaimEndAt != row.EndAt ||
				period.StartAt != row.StartAt || period.EndAt != row.EndAt || period.RewardEndAt != row.ClaimEndAt ||
				!feedback78FormValid(row.FormJSON) || i > 0 && occurrences[i-1].EndAt != row.StartAt {
				return fmt.Errorf("%w: 排期规则不符 %s", constUtil.ErrAdminConfigConflict, row.ID)
			}
		}
		var template db.AdminActivityDefinition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&template, "id=?", constUtil.Feedback78TemplateID).Error; err != nil {
			return err
		}
		if template.Name != constUtil.Feedback78ActivityName || template.Mode != constUtil.Feedback78ActivityMode || !feedback78FormValid(template.FormJSON) {
			return fmt.Errorf("%w: 模板规则不符", constUtil.ErrAdminConfigConflict)
		}
		var vip db.VIPShopProduct
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&vip, "id=?", constUtil.P3VIP10GrowthProduct).Error; err != nil {
			return err
		}
		if vip.MinVIP != 10 || vip.Reward != "item_044,1" || vip.CostResource != "diamond" || !vip.Enabled || vip.TotalLimit != 0 ||
			vip.Cost != 1100 && vip.Cost != 2888 || vip.Source != constUtil.P3VIP10GrowthSource && vip.Source != constUtil.Feedback78VIPCurrentSource ||
			vip.Cost == 1100 && vip.Source != constUtil.P3VIP10GrowthSource {
			return fmt.Errorf("%w: VIP10商品偏离审核值", constUtil.ErrAdminConfigConflict)
		}
		changed, scheduleChanged := false, false
		if !template.Archived || template.Enabled {
			if err := tx.Model(&db.AdminActivityDefinition{}).Where("id=?", template.ID).
				Updates(map[string]any{"archived": true, "enabled": false, "revision": template.Revision + 1, "updated_at_millis": realNow.UnixMilli()}).Error; err != nil {
				return err
			}
			changed = true
		}
		for _, row := range occurrences {
			if !row.Archived || row.Enabled {
				if err := tx.Model(&db.AdminActivityOccurrence{}).Where("id=?", row.ID).
					Updates(map[string]any{"archived": true, "enabled": false, "revision": row.Revision + 1}).Error; err != nil {
					return err
				}
				changed = true
				scheduleChanged = true
			}
		}
		for _, period := range periods {
			if period.Enabled {
				if err := tx.Model(&db.PeriodInstance{}).Where("id=?", period.ID).Update("enabled", false).Error; err != nil {
					return err
				}
				changed = true
			}
		}
		sources := make([]string, 0, len(periods))
		for _, period := range periods {
			sources = append(sources, p1PeriodSources(period)...)
		}
		retired, err := feedback78RetireDeliveries(tx, sources, now, ceiling)
		if err != nil {
			return err
		}
		if retired > 0 {
			changed = true
		}
		if vip.Cost != 2888 || vip.Source != constUtil.Feedback78VIPCurrentSource {
			if err := tx.Model(&db.VIPShopProduct{}).Where("id=?", vip.ID).
				Updates(map[string]any{"cost": 2888, "source": constUtil.Feedback78VIPCurrentSource}).Error; err != nil {
				return err
			}
			changed = true
		}
		if !changed {
			return nil
		}
		if scheduleChanged {
			if err := tx.Model(&db.AdminActivityScheduleRevision{}).Where("server_id=?", serverID).Update("revision", revision.Revision+1).Error; err != nil {
				return err
			}
		}
		auditor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
		return adminAudit(tx, auditor, constUtil.Feedback78AuditAction, fmt.Sprint(serverID),
			fmt.Sprintf("template=%s; schedules=%d; retired=%d; expiry=%d; vip=%s; previous_cost=%d; previous_source=%s", template.ID, len(occurrences), retired, now, vip.ID, vip.Cost, vip.Source))
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

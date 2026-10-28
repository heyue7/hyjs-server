package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// feedback81Form validates the complete rollout contract and changes only its first reward.
func feedback81Form(raw []byte) ([]byte, bool, error) {
	var f model.AdminActivityForm
	if json.Unmarshal(raw, &f) != nil || f.Mode != constUtil.Feedback81Mode || f.TemplateID != constUtil.Feedback81ArtTemplate ||
		(f.Name != constUtil.Feedback81Name && f.Name != constUtil.Feedback81OriginalName) || f.Source != constUtil.Feedback81Source ||
		f.Alias != constUtil.Feedback81Alias || f.RepeatPolicy != "repeat" || len(f.Products) != 0 || len(f.Prizes) != 0 ||
		!bytes.Equal(adminJSON(f.Options), adminJSON(model.AdminActivityOptions{})) || len(f.Milestones) != 3 {
		return nil, false, fmt.Errorf("%w: 登录活动表单身份不符", constUtil.ErrAdminConfigConflict)
	}
	old := []model.AdminResourceAmount{{ResourceID: "item_9760", Amount: 1}}
	next := []model.AdminResourceAmount{{ResourceID: "item_9760", Amount: 1}, {ResourceID: "equip_041", Amount: 1}}
	second := []model.AdminResourceAmount{{ResourceID: "equip_025", Amount: 1}, {ResourceID: "equip_026", Amount: 1}}
	third := []model.AdminResourceAmount{{ResourceID: "equip_027", Amount: 1}, {ResourceID: "equip_028", Amount: 1}}
	for i, rewards := range [][]model.AdminResourceAmount{f.Milestones[0].Rewards, second, third} {
		if f.Milestones[i].Goal != int64(i+1) || len(f.Milestones[i].Options) != 0 || !reflect.DeepEqual(f.Milestones[i].Rewards, rewards) {
			return nil, false, fmt.Errorf("%w: 登录活动档位不符", constUtil.ErrAdminConfigConflict)
		}
	}
	if reflect.DeepEqual(f.Milestones[0].Rewards, next) {
		return raw, false, nil
	}
	if !reflect.DeepEqual(f.Milestones[0].Rewards, old) {
		return nil, false, fmt.Errorf("%w: 登录活动首档不符", constUtil.ErrAdminConfigConflict)
	}
	var object map[string]json.RawMessage
	var tiers []map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || json.Unmarshal(object["milestones"], &tiers) != nil || len(tiers) != 3 || tiers[0] == nil {
		return nil, false, constUtil.ErrAdminConfiguration
	}
	tiers[0]["rewards"] = adminJSON(next)
	object["milestones"] = adminJSON(tiers)
	return adminJSON(object), true, nil
}

// feedback81Period checks both historical source formats without changing their keys.
func feedback81Period(cfg db.PeriodInstance, issued db.AdminCharacterActivityInstance) error {
	if cfg.ID != issued.ConsumerKey || cfg.Title != constUtil.Feedback81Name || cfg.Source != constUtil.Feedback81Source ||
		cfg.Kind != constUtil.P1EventLoginDay || cfg.Mode != "cumulative" || cfg.Goal != 1 || cfg.RewardSourceID != "" ||
		cfg.StartAt != issued.StartAt || cfg.EndAt != issued.EndAt || cfg.RewardEndAt != issued.ClaimEndAt ||
		cfg.RankPlaces != 0 || cfg.RankRewards != "" {
		return fmt.Errorf("%w: 登录活动期次身份不符 %s", constUtil.ErrAdminConfigConflict, issued.ConsumerKey)
	}
	tiers := p1PeriodMilestones(cfg)
	if len(tiers) != 3 || cfg.Rewards != tiers[0].Rewards ||
		(tiers[0].Rewards != constUtil.Feedback81OldReward && tiers[0].Rewards != constUtil.Feedback81NewReward) ||
		tiers[1].Rewards != "equip_025,1;equip_026,1" || tiers[2].Rewards != "equip_027,1;equip_028,1" {
		return fmt.Errorf("%w: 登录活动期次奖励不符 %s", constUtil.ErrAdminConfigConflict, issued.ConsumerKey)
	}
	legacySources := tiers[0].SourceKey != ""
	for i, tier := range tiers {
		if tier.Goal != int64(i+1) || (legacySources && tier.SourceKey != activity77PeriodSource(cfg.ID, i)) || (!legacySources && tier.SourceKey != "") {
			return fmt.Errorf("%w: 登录活动期次来源不符 %s", constUtil.ErrAdminConfigConflict, issued.ConsumerKey)
		}
	}
	return nil
}

// Frozen and progress snapshots can retain an older enabled flag, but all other
// fields and each tier source must agree with the live period.
func feedback81PeriodEquivalent(a, b db.PeriodInstance) bool {
	normalize := func(v db.PeriodInstance) db.PeriodInstance {
		v.Frozen = nil
		v.Enabled = false
		v.Rewards = constUtil.Feedback81OldReward
		tiers := p1PeriodMilestones(v)
		if len(tiers) > 0 {
			tiers[0].Rewards = constUtil.Feedback81OldReward
		}
		v.Milestones = adminJSON(tiers)
		return v
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}

func feedback81PatchPeriod(cfg db.PeriodInstance) db.PeriodInstance {
	var tiers []map[string]json.RawMessage
	_ = json.Unmarshal(cfg.Milestones, &tiers) // The caller has validated all three tiers.
	tiers[0]["rewards"] = adminJSON(constUtil.Feedback81NewReward)
	cfg.Rewards = constUtil.Feedback81NewReward
	cfg.Milestones = adminJSON(tiers)
	return cfg
}

// Preserve fields outside the changed reward in a saved JSON snapshot.
func feedback81PatchSnapshot(raw []byte, cfg db.PeriodInstance) ([]byte, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, constUtil.ErrAdminConfiguration
	}
	updated := feedback81PatchPeriod(cfg)
	object["rewards"] = adminJSON(updated.Rewards)
	object["milestones"] = adminJSON(updated.Milestones)
	return adminJSON(object), nil
}

type feedback81Instance struct {
	issued  db.AdminCharacterActivityInstance
	period  db.PeriodInstance
	sources []string
}

// ApplyFeedback81Config is a bounded maintenance operation for the verified
// realm-one rollout. It never grants rewards or creates deliveries.
func (s *GameService) ApplyFeedback81Config(ctx context.Context, actor *model.AdminPrincipal) error {
	if s.OriginalServerID() != 1 {
		return constUtil.ErrAdminConfiguration
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var user db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, actor.UserID).Error; err != nil || !user.Enabled || user.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		var policy db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Maintenance {
			return fmt.Errorf("%w: 须在维护期执行", constUtil.ErrAdminConfigConflict)
		}
		if s.ServerClockReady {
			var clock db.ServerGameClock
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&clock, "server_id=?", 1).Error; err != nil || !serverGameClockValid(&clock) {
				return constUtil.ErrAdminConfiguration
			}
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var occurrences []db.AdminActivityOccurrence
		if err := tx.Where("template_id=?", constUtil.Feedback81TemplateID).Order("id").Limit(constUtil.Feedback81MaxOccurrences + 1).Find(&occurrences).Error; err != nil {
			return err
		}
		if len(occurrences) != 1 || occurrences[0].ID != constUtil.Feedback81OccurrenceID || occurrences[0].ServerID != 1 {
			return fmt.Errorf("%w: 登录活动排期或区服引用不符", constUtil.ErrAdminConfigConflict)
		}
		// occurrence_id is not an index prefix. Cap the primary-key scan before
		// locating the target rows, even if very few of them match this schedule.
		var scanProbe []db.AdminCharacterActivityInstance
		if err := tx.Select("player_id", "occurrence_id").Order("player_id,occurrence_id").Offset(constUtil.Feedback81MaxCharacterScan).Limit(1).Find(&scanProbe).Error; err != nil {
			return err
		}
		if len(scanProbe) > 0 {
			return fmt.Errorf("%w: 角色活动扫描超过维护上限", constUtil.ErrAdminConfigConflict)
		}
		var initial []db.AdminCharacterActivityInstance
		if err := tx.Where("occurrence_id=?", constUtil.Feedback81OccurrenceID).Order("player_id").Limit(constUtil.Feedback81MaxInstances + 1).Find(&initial).Error; err != nil {
			return err
		}
		if len(initial) > constUtil.Feedback81MaxInstances {
			return fmt.Errorf("%w: 登录活动实例超过维护上限", constUtil.ErrAdminConfigConflict)
		}
		playerIDs := make([]int64, 0, len(initial))
		periodIDs := make([]string, 0, len(initial))
		for _, row := range initial {
			if row.PlayerID <= 0 || row.ServerID != 1 || row.Mode != constUtil.Feedback81Mode || row.ConsumerKey == "" || row.ConsumerID != "period:"+row.ConsumerKey {
				return fmt.Errorf("%w: 登录活动实例归属不符", constUtil.ErrAdminConfigConflict)
			}
			playerIDs = append(playerIDs, row.PlayerID)
			periodIDs = append(periodIDs, row.ConsumerKey)
		}
		sort.Slice(playerIDs, func(i, j int) bool { return playerIDs[i] < playerIDs[j] })
		sort.Strings(periodIDs)
		for i := 1; i < len(periodIDs); i++ {
			if periodIDs[i] == periodIDs[i-1] {
				return constUtil.ErrAdminConfigConflict
			}
		}
		// Follow the gameplay lock order: clock, all affected profiles, periods, then schedule.
		for start := 0; start < len(playerIDs); start += constUtil.Feedback81BatchSize {
			end := min(start+constUtil.Feedback81BatchSize, len(playerIDs))
			var profiles []db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "server_id").Where("id IN ?", playerIDs[start:end]).Order("id").Find(&profiles).Error; err != nil {
				return err
			}
			if len(profiles) != end-start {
				return fmt.Errorf("%w: 登录活动角色缺失", constUtil.ErrAdminConfigConflict)
			}
			for _, profile := range profiles {
				if AdminProfileServerID(profile) != 1 {
					return constUtil.ErrAdminConfigConflict
				}
			}
		}
		periods := make(map[string]db.PeriodInstance, len(periodIDs))
		for start := 0; start < len(periodIDs); start += constUtil.Feedback81BatchSize {
			end := min(start+constUtil.Feedback81BatchSize, len(periodIDs))
			var rows []db.PeriodInstance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", periodIDs[start:end]).Order("id").Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) != end-start {
				return fmt.Errorf("%w: 登录活动期次缺失", constUtil.ErrAdminConfigConflict)
			}
			for _, row := range rows {
				periods[row.ID] = row
			}
		}
		var revision db.AdminActivityScheduleRevision
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&revision, "server_id=?", 1).Error; err != nil {
			return err
		}
		var occurrence db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&occurrence, "id=?", constUtil.Feedback81OccurrenceID).Error; err != nil {
			return err
		}
		if !reflect.DeepEqual(occurrence, occurrences[0]) || occurrence.Archived || occurrence.TemplateID != constUtil.Feedback81TemplateID ||
			occurrence.Mode != constUtil.Feedback81Mode || occurrence.Name != constUtil.Feedback81Name || occurrence.Timing != "character-day" ||
			occurrence.StartDay != 1 || occurrence.DurationDays != 28 || occurrence.ClaimDays != 0 || occurrence.Lane != 2 {
			return fmt.Errorf("%w: 登录活动排期已变化", constUtil.ErrAdminConfigConflict)
		}
		var current []db.AdminCharacterActivityInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("occurrence_id=?", occurrence.ID).Order("player_id").Limit(constUtil.Feedback81MaxInstances + 1).Find(&current).Error; err != nil {
			return err
		}
		if !reflect.DeepEqual(initial, current) {
			return fmt.Errorf("%w: 登录活动实例已变化，请重试", constUtil.ErrAdminConfigConflict)
		}
		var definition db.AdminActivityDefinition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&definition, "id=?", constUtil.Feedback81TemplateID).Error; err != nil {
			return err
		}
		if definition.Archived || definition.Name != constUtil.Feedback81Name || definition.Mode != constUtil.Feedback81Mode {
			return fmt.Errorf("%w: 登录活动模板身份不符", constUtil.ErrAdminConfigConflict)
		}
		// A current read under lock catches foreign references added since the initial scan.
		var references []db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("template_id=?", definition.ID).Order("id").Limit(constUtil.Feedback81MaxOccurrences + 1).Find(&references).Error; err != nil {
			return err
		}
		if len(references) != 1 || !reflect.DeepEqual(references[0], occurrence) {
			return fmt.Errorf("%w: 登录活动模板引用已变化", constUtil.ErrAdminConfigConflict)
		}
		newDefinition, definitionChanged, err := feedback81Form(definition.FormJSON)
		if err != nil {
			return err
		}
		newOccurrence, occurrenceChanged, err := feedback81Form(occurrence.FormJSON)
		if err != nil {
			return err
		}
		instances := make([]feedback81Instance, 0, len(current))
		byPeriod := make(map[string]feedback81Instance, len(current))
		for _, issued := range current {
			var form model.AdminActivityForm
			if json.Unmarshal(issued.FormJSON, &form) != nil || form.StartAt != issued.StartAt || form.EndAt != issued.EndAt || form.ClaimEndAt != issued.ClaimEndAt {
				return fmt.Errorf("%w: 角色活动日期不符", constUtil.ErrAdminConfigConflict)
			}
			if _, _, err := feedback81Form(issued.FormJSON); err != nil {
				return err
			}
			period := periods[issued.ConsumerKey]
			if err := feedback81Period(period, issued); err != nil {
				return err
			}
			if err := s.validateP1Period(feedback81PatchPeriod(period)); err != nil {
				return err
			}
			if len(period.Frozen) > 0 {
				var frozen db.PeriodInstance
				if json.Unmarshal(period.Frozen, &frozen) != nil || feedback81Period(frozen, issued) != nil || !feedback81PeriodEquivalent(period, frozen) {
					return fmt.Errorf("%w: 活动冻结期次不符 %s", constUtil.ErrAdminConfigConflict, issued.ConsumerKey)
				}
			}
			item := feedback81Instance{issued: issued, period: period, sources: p1PeriodSources(period)}
			instances = append(instances, item)
			byPeriod[period.ID] = item
		}
		changed := 0
		rewardIDs := make(map[string]int64, len(instances))
		for start := 0; start < len(periodIDs); start += constUtil.Feedback81BatchSize {
			end := min(start+constUtil.Feedback81BatchSize, len(periodIDs))
			var progress []db.PeriodProgress
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id IN ?", periodIDs[start:end]).Order("instance_id,player_id").Limit(constUtil.Feedback81BatchSize + 1).Find(&progress).Error; err != nil {
				return err
			}
			if len(progress) > end-start {
				return fmt.Errorf("%w: 活动进度归属或数量不符", constUtil.ErrAdminConfigConflict)
			}
			for _, row := range progress {
				item, ok := byPeriod[row.InstanceID]
				var snapshot db.PeriodInstance
				if !ok || row.PlayerID != item.issued.PlayerID || row.Value < 0 || json.Unmarshal(row.Snapshot, &snapshot) != nil ||
					feedback81Period(snapshot, item.issued) != nil || !feedback81PeriodEquivalent(snapshot, item.period) {
					return fmt.Errorf("%w: 活动进度快照不符 %s/%d", constUtil.ErrAdminConfigConflict, row.InstanceID, row.PlayerID)
				}
				rewardIDs[row.InstanceID] = row.RewardID
				if snapshot.Rewards == constUtil.Feedback81OldReward {
					patched, err := feedback81PatchSnapshot(row.Snapshot, snapshot)
					if err != nil {
						return err
					}
					result := tx.Model(&db.PeriodProgress{}).Where("instance_id=? AND player_id=? AND snapshot=?", row.InstanceID, row.PlayerID, row.Snapshot).Update("snapshot", patched)
					if result.Error != nil || result.RowsAffected != 1 {
						return constUtil.ErrAdminConfigConflict
					}
					changed++
				}
			}
		}
		for start := 0; start < len(instances); start += constUtil.Feedback81BatchSize {
			end := min(start+constUtil.Feedback81BatchSize, len(instances))
			players, sources := []int64{}, []string{}
			type receiptTarget struct {
				playerID int64
				periodID string
				tier     int
			}
			targets := map[string]receiptTarget{}
			seenFirst := map[string]int64{}
			for _, item := range instances[start:end] {
				players = append(players, item.issued.PlayerID)
				for tier, source := range item.sources {
					if _, exists := targets[source]; exists {
						return constUtil.ErrAdminConfigConflict
					}
					targets[source] = receiptTarget{playerID: item.issued.PlayerID, periodID: item.period.ID, tier: tier}
					sources = append(sources, source)
				}
			}
			var deliveries []db.RewardDelivery
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id IN ? AND source_key IN ?", players, sources).Order("player_id,source_key").Limit(3*constUtil.Feedback81BatchSize + 1).Find(&deliveries).Error; err != nil {
				return err
			}
			if len(deliveries) > 3*(end-start) {
				return constUtil.ErrAdminConfigConflict
			}
			for _, delivery := range deliveries {
				target, exists := targets[delivery.SourceKey]
				if !exists || target.playerID != delivery.PlayerID {
					return constUtil.ErrAdminConfigConflict
				}
				if target.tier != 0 {
					continue // Other tiers stay intact.
				}
				seenFirst[target.periodID] = delivery.ID
				if delivery.Rewards != constUtil.Feedback81OldReward && delivery.Rewards != constUtil.Feedback81NewReward {
					return fmt.Errorf("%w: 首日领奖回执不符", constUtil.ErrAdminConfigConflict)
				}
				if delivery.ClaimedAt == 0 && delivery.Rewards == constUtil.Feedback81OldReward {
					result := tx.Model(&db.RewardDelivery{}).Where("id=? AND claimed_at=0 AND rewards=?", delivery.ID, constUtil.Feedback81OldReward).Update("rewards", constUtil.Feedback81NewReward)
					if result.Error != nil || result.RowsAffected != 1 {
						return constUtil.ErrAdminConfigConflict
					}
					changed++
				}
			}
			for _, item := range instances[start:end] {
				if rewardIDs[item.period.ID] != 0 && rewardIDs[item.period.ID] != seenFirst[item.period.ID] {
					return fmt.Errorf("%w: 活动领奖引用不符", constUtil.ErrAdminConfigConflict)
				}
			}
		}
		for _, item := range instances {
			patchedForm, formChanged, err := feedback81Form(item.issued.FormJSON)
			if err != nil {
				return err
			}
			if formChanged {
				result := tx.Model(&db.AdminCharacterActivityInstance{}).Where("player_id=? AND occurrence_id=? AND form_json=?", item.issued.PlayerID, occurrence.ID, item.issued.FormJSON).Update("form_json", patchedForm)
				if result.Error != nil || result.RowsAffected != 1 {
					return constUtil.ErrAdminConfigConflict
				}
				changed++
			}
			period := item.period
			updates := map[string]any{}
			if period.Rewards == constUtil.Feedback81OldReward {
				patched := feedback81PatchPeriod(period)
				updates["rewards"], updates["milestones"] = patched.Rewards, patched.Milestones
			}
			if len(period.Frozen) > 0 {
				var frozen db.PeriodInstance
				if json.Unmarshal(period.Frozen, &frozen) != nil {
					return constUtil.ErrAdminConfiguration
				}
				if frozen.Rewards == constUtil.Feedback81OldReward {
					patched, err := feedback81PatchSnapshot(period.Frozen, frozen)
					if err != nil {
						return err
					}
					updates["frozen"] = patched
				}
			}
			if len(updates) > 0 {
				if err := tx.Model(&db.PeriodInstance{}).Where("id=?", period.ID).Updates(updates).Error; err != nil {
					return err
				}
				changed++
			}
		}
		if occurrenceChanged {
			result := tx.Model(&db.AdminActivityOccurrence{}).Where("id=? AND revision=?", occurrence.ID, occurrence.Revision).Updates(map[string]any{"form_json": newOccurrence, "revision": occurrence.Revision + 1})
			if result.Error != nil || result.RowsAffected != 1 {
				return constUtil.ErrAdminConfigConflict
			}
			if err := tx.Model(&db.AdminActivityScheduleRevision{}).Where("server_id=? AND revision=?", 1, revision.Revision).Update("revision", revision.Revision+1).Error; err != nil {
				return err
			}
			changed++
		}
		if definitionChanged {
			result := tx.Model(&db.AdminActivityDefinition{}).Where("id=? AND revision=?", definition.ID, definition.Revision).Updates(map[string]any{"form_json": newDefinition, "revision": definition.Revision + 1, "updated_at_millis": time.Now().UnixMilli()})
			if result.Error != nil || result.RowsAffected != 1 {
				return constUtil.ErrAdminConfigConflict
			}
			changed++
		}
		var audits int64
		if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.Feedback81AuditAction, occurrence.ID).Count(&audits).Error; err != nil {
			return err
		}
		if changed > 0 || audits == 0 {
			return adminAudit(tx, actor, constUtil.Feedback81AuditAction, occurrence.ID, fmt.Sprintf("三日登录首档追加荣耀之刃；核验实例=%d，修改=%d；已领回执及角色资产保持", len(instances), changed))
		}
		return nil
	})
}

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// These original forms constrain the maintenance migration to the known rollout.
var activity77PeriodOldForms = []string{
	`{"mode":"login-total","templateId":"login-total-original","name":"启程装备补给","source":"成长投放V12 / 2026-10-26","alias":"成长投放V12","repeatPolicy":"repeat","products":[],"milestones":[{"goal":1,"rewards":[{"resourceId":"equip_025","amount":1},{"resourceId":"equip_026","amount":1},{"resourceId":"equip_027","amount":1},{"resourceId":"equip_028","amount":1}]}],"prizes":[],"options":{}}`,
	`{"mode":"wanted-attempts","templateId":"wanted-attempts-original","name":"每日悬赏挑战","source":"成长投放V12 / 2026-10-26","alias":"成长投放V12","repeatPolicy":"repeat","products":[],"milestones":[{"goal":2,"rewards":[{"resourceId":"coin","amount":4000000},{"resourceId":"score","amount":2100}]},{"goal":4,"rewards":[{"resourceId":"coin","amount":10000000}]},{"goal":6,"rewards":[{"resourceId":"coin","amount":6000000},{"resourceId":"score","amount":900},{"resourceId":"item_374","amount":200}]}],"prizes":[],"options":{}}`,
	`{"mode":"trial-stars","templateId":"trial-stars-original","name":"每周试炼挑战","source":"成长投放V12 / 2026-10-26","alias":"成长投放V12","repeatPolicy":"repeat","products":[],"milestones":[{"goal":27,"rewards":[{"resourceId":"coin","amount":60000000}]}],"prizes":[],"options":{}}`,
	`{"mode":"wanted-attempts","templateId":"wanted-attempts-original","name":"每周悬赏成长","source":"成长投放V12 / 2026-10-26","alias":"成长投放V12","repeatPolicy":"repeat","products":[],"milestones":[{"goal":20,"rewards":[{"resourceId":"item_460","amount":36},{"resourceId":"item_459","amount":550},{"resourceId":"item_461","amount":20},{"resourceId":"item_462","amount":46},{"resourceId":"item_463","amount":54},{"resourceId":"MineItem_083","amount":4085},{"resourceId":"MineItem_013","amount":3801},{"resourceId":"MineItem_084","amount":48838},{"resourceId":"MineItem_014","amount":1215},{"resourceId":"MineItem_015","amount":1215},{"resourceId":"MineItem_016","amount":1215},{"resourceId":"MineItem_017","amount":810},{"resourceId":"MineItem_018","amount":810},{"resourceId":"MineItem_081","amount":170},{"resourceId":"MineItem_082","amount":520},{"resourceId":"FishItem_007","amount":6},{"resourceId":"FishItem_008","amount":42},{"resourceId":"FishItem_009","amount":165},{"resourceId":"FishItem_010","amount":125},{"resourceId":"FishItem_011","amount":120},{"resourceId":"FishItem_012","amount":100},{"resourceId":"FishItem_018","amount":100},{"resourceId":"FishItem_025","amount":20},{"resourceId":"frag_trea_011_1","amount":1},{"resourceId":"frag_trea_011_2","amount":1},{"resourceId":"frag_trea_011_3","amount":1},{"resourceId":"frag_trea_011_4","amount":1},{"resourceId":"frag_trea_011_5","amount":1},{"resourceId":"frag_trea_012_1","amount":1},{"resourceId":"frag_trea_012_2","amount":1},{"resourceId":"frag_trea_012_3","amount":1},{"resourceId":"frag_trea_012_4","amount":1},{"resourceId":"frag_trea_012_5","amount":1},{"resourceId":"frag_trea_013_1","amount":1},{"resourceId":"frag_trea_013_2","amount":1},{"resourceId":"frag_trea_013_3","amount":1},{"resourceId":"frag_trea_013_4","amount":1},{"resourceId":"frag_trea_013_5","amount":1},{"resourceId":"MineItem_009","amount":200},{"resourceId":"FishItem_015","amount":40},{"resourceId":"FishItem_021","amount":40},{"resourceId":"item_002","amount":2},{"resourceId":"item_005","amount":1000},{"resourceId":"item_006","amount":25000}]},{"goal":40,"rewards":[{"resourceId":"item_005","amount":1000},{"resourceId":"item_006","amount":25000}]}],"prizes":[],"options":{}}`,
}

type activity77PeriodPair struct{ old, next model.AdminActivityForm }

func activity77ComparableForm(f model.AdminActivityForm) model.AdminActivityForm {
	f = adminActivityCanonicalForm(f)
	f.StartAt = 0
	f.EndAt = 0
	f.ClaimEndAt = 0
	return f
}
func activity77PeriodPairs() ([]activity77PeriodPair, error) {
	var input []growth74Activity
	if err := json.Unmarshal(growth74ActivitiesJSON, &input); err != nil {
		return nil, err
	}
	out := make([]activity77PeriodPair, 0, len(activity77PeriodOldForms))
	for _, raw := range activity77PeriodOldForms {
		var old model.AdminActivityForm
		if err := json.Unmarshal([]byte(raw), &old); err != nil {
			return nil, err
		}
		var next *model.AdminActivityForm
		for _, item := range input {
			if item.Form.Name == old.Name && item.Form.Mode == old.Mode {
				x := item.Form
				if next != nil && !reflect.DeepEqual(activity77ComparableForm(*next), activity77ComparableForm(x)) {
					return nil, fmt.Errorf("目标活动配置不一致: %s", old.Name)
				}
				next = &x
			}
		}
		if next == nil {
			return nil, fmt.Errorf("目标活动缺失: %s", old.Name)
		}
		out = append(out, activity77PeriodPair{activity77ComparableForm(old), activity77ComparableForm(*next)})
	}
	return out, nil
}
func activity77FormMatch(f model.AdminActivityForm, expected model.AdminActivityForm) bool {
	return reflect.DeepEqual(activity77ComparableForm(f), expected)
}
func activity77ChangeForm(raw []byte, pair activity77PeriodPair) ([]byte, error) {
	var f model.AdminActivityForm
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if !activity77FormMatch(f, pair.old) {
		return nil, fmt.Errorf("活动表单与原投放不符")
	}
	f.Milestones = pair.next.Milestones
	return adminJSON(f), nil
}
func (s *GameService) activity77PeriodTiers(f model.AdminActivityForm, id string, source bool) ([]p1PeriodMilestone, error) {
	tiers := make([]p1PeriodMilestone, 0, len(f.Milestones))
	for i, m := range f.Milestones {
		reward, err := s.adminTypedResourceSpec(m.Rewards, false)
		if err != nil {
			return nil, err
		}
		t := p1PeriodMilestone{Goal: m.Goal, Rewards: reward}
		if source {
			t.SourceKey = activity77PeriodSource(id, i)
		}
		tiers = append(tiers, t)
	}
	return tiers, nil
}
func activity77PeriodEqual(a, b db.PeriodInstance) bool {
	a.Frozen = nil
	b.Frozen = nil
	a.Enabled = b.Enabled
	return reflect.DeepEqual(a, b)
}

// ApplyActivity77Periods updates only exact V12 activity rules during maintenance.
func (s *GameService) ApplyActivity77Periods(ctx context.Context, actor *model.AdminPrincipal) error {
	d := db.GetIns().WithContext(ctx)
	var policy db.ClientUpdatePolicy
	if err := d.First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Maintenance {
		return fmt.Errorf("先关服再迁移活动")
	}
	if err := adminOperationUser(d, actor, true); err != nil {
		return err
	}
	pairs, err := activity77PeriodPairs()
	if err != nil {
		return err
	}
	var definitions []db.AdminActivityDefinition
	if err = d.Where("archived=?", false).Find(&definitions).Error; err != nil {
		return err
	}
	// A completed template is accepted only with its own committed migration audit.
	matched := make([]bool, len(pairs))
	for _, definition := range definitions {
		var form model.AdminActivityForm
		if json.Unmarshal(definition.FormJSON, &form) != nil {
			return constUtil.ErrAdminConfiguration
		}
		for i, pair := range pairs {
			if activity77FormMatch(form, pair.old) {
				matched[i] = true
				continue
			}
			if activity77FormMatch(form, pair.next) {
				var audit int64
				if err = d.Model(&db.AdminAudit{}).Where("action=? AND target=?", "activity77.period.template", definition.ID).Count(&audit).Error; err != nil {
					return err
				}
				if audit == 0 {
					return fmt.Errorf("目标活动模板 %s 缺少迁移审计", definition.ID)
				}
				matched[i] = true
			}
		}
	}
	labels := []string{"启程登录", "每日悬赏", "每周试炼", "每周悬赏"}
	for i, found := range matched {
		if !found {
			return fmt.Errorf("未找到目标活动模板: %s", labels[i])
		}
	}
	// Check shared templates before any migration is committed.
	for _, def := range definitions {
		var form model.AdminActivityForm
		if json.Unmarshal(def.FormJSON, &form) != nil {
			return constUtil.ErrAdminConfiguration
		}
		for _, pair := range pairs {
			if activity77FormMatch(form, pair.old) {
				var count int64
				if err = d.Model(&db.AdminActivityOccurrence{}).Where("server_id<>? AND template_id=? AND archived=?", s.OriginalServerID(), def.ID, false).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					return fmt.Errorf("模板 %s 被其他区服引用", def.ID)
				}
			}
		}
	}
	for _, def := range definitions {
		var f model.AdminActivityForm
		if json.Unmarshal(def.FormJSON, &f) != nil {
			return constUtil.ErrAdminConfiguration
		}
		for _, pair := range pairs {
			if !activity77FormMatch(f, pair.old) {
				continue
			}
			var foreign int64
			if err = d.Model(&db.AdminActivityOccurrence{}).Where("server_id<>? AND template_id=? AND archived=?", s.OriginalServerID(), def.ID, false).Count(&foreign).Error; err != nil {
				return err
			}
			if foreign > 0 {
				return fmt.Errorf("模板 %s 被其他区服引用", def.ID)
			}
			var occurrences []db.AdminActivityOccurrence
			if err = d.Where("server_id=? AND template_id=? AND archived=?", s.OriginalServerID(), def.ID, false).Order("id").Find(&occurrences).Error; err != nil {
				return err
			}
			for _, occ := range occurrences {
				if adminActivityTiming(occ) != "character-day" && occ.ClaimEndAt <= time.Now().UnixMilli() {
					continue
				}
				if err = s.activity77PeriodOccurrence(ctx, actor, occ, pair); err != nil {
					return fmt.Errorf("活动 %s: %w", occ.ID, err)
				}
			}
			if err = d.Transaction(func(tx *gorm.DB) error {
				var row db.AdminActivityDefinition
				if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", def.ID).Error; e != nil {
					return e
				}
				var foreign int64
				if e := tx.Model(&db.AdminActivityOccurrence{}).Where("server_id<>? AND template_id=? AND archived=?", s.OriginalServerID(), row.ID, false).Count(&foreign).Error; e != nil {
					return e
				}
				if foreign > 0 {
					return fmt.Errorf("模板 %s 被其他区服引用", row.ID)
				}
				if !bytes.Equal(row.FormJSON, def.FormJSON) {
					return constUtil.ErrAdminConfigConflict
				}
				newForm, e := activity77ChangeForm(row.FormJSON, pair)
				if e != nil {
					return e
				}
				row.FormJSON = newForm
				row.Revision++
				row.UpdatedAtMillis = time.Now().UnixMilli()
				if e = tx.Save(&row).Error; e != nil {
					return e
				}
				return adminAudit(tx, actor, "activity77.period.template", row.ID, "更新成长投放模板的奖励档位")
			}); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (s *GameService) activity77PeriodOccurrence(ctx context.Context, actor *model.AdminPrincipal, occurrence db.AdminActivityOccurrence, pair activity77PeriodPair) error {
	d := db.GetIns().WithContext(ctx)
	if adminActivityTiming(occurrence) == "character-day" {
		var issued []db.AdminCharacterActivityInstance
		if err := d.Where("occurrence_id=?", occurrence.ID).Order("player_id").Limit(int(constUtil.Activity77RestaurantStateLimit) + 1).Find(&issued).Error; err != nil {
			return err
		}
		if len(issued) > int(constUtil.Activity77RestaurantStateLimit) {
			return fmt.Errorf("已发行活动超过单次上限")
		}
		for _, row := range issued {
			if err := d.Transaction(func(tx *gorm.DB) error {
				if e := ServerGameClockGuardTransaction(tx); e != nil {
					return e
				}
				var profile db.Profile
				if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&profile, row.PlayerID).Error; e != nil {
					return e
				}
				var current db.AdminCharacterActivityInstance
				if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND occurrence_id=?", row.PlayerID, row.OccurrenceID).First(&current).Error; e != nil {
					return e
				}
				var audit int64
				if e := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", "activity77.period.consumer", current.ConsumerKey).Count(&audit).Error; e != nil {
					return e
				}
				if audit > 0 {
					return nil
				}
				form, e := activity77ChangeForm(current.FormJSON, pair)
				if e != nil {
					return e
				}
				if e = s.activity77MigratePeriod(tx, actor, current.ConsumerKey, pair, current.StartAt, current.EndAt, current.ClaimEndAt); e != nil {
					return e
				}
				return tx.Model(&current).Update("form_json", form).Error
			}); err != nil {
				return err
			}
		}
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if e := ServerGameClockGuardTransaction(tx); e != nil {
			return e
		}
		guard, e := adminActivityScheduleRevision(tx, occurrence.ServerID, true)
		if e != nil {
			return e
		}
		var row db.AdminActivityOccurrence
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", occurrence.ID).Error; e != nil {
			return e
		}
		var form model.AdminActivityForm
		if json.Unmarshal(row.FormJSON, &form) != nil {
			return constUtil.ErrAdminConfiguration
		}
		if activity77FormMatch(form, pair.next) {
			return nil
		}
		patched, e := activity77ChangeForm(row.FormJSON, pair)
		if e != nil {
			return e
		}
		if adminActivityTiming(row) != "character-day" {
			if e = s.activity77MigratePeriod(tx, actor, row.ConsumerKey, pair, row.StartAt, row.EndAt, row.ClaimEndAt); e != nil {
				return e
			}
		}
		row.FormJSON = patched
		row.Revision++
		if e = tx.Save(&row).Error; e != nil {
			return e
		}
		guard.Revision++
		if e = tx.Save(&guard).Error; e != nil {
			return e
		}
		return adminAudit(tx, actor, "activity77.period.occurrence", row.ID, "迁移成长投放活动档位")
	})
}

func activity77PeriodCovered(pair activity77PeriodPair, oldIndex int) []int {
	switch pair.old.Mode {
	case "login-total":
		return []int{1, 2}
	case "trial-stars":
		return []int{0, 1, 2, 3, 4}
	case "wanted-attempts":
		if len(pair.old.Milestones) == 3 {
			return []int{oldIndex}
		}
		if oldIndex == 0 {
			return []int{0, 1, 2, 3}
		}
		if oldIndex == 1 {
			return []int{4}
		}
	}
	return nil
}

func (s *GameService) activity77MigratePeriod(tx *gorm.DB, actor *model.AdminPrincipal, id string, pair activity77PeriodPair, start, end, claim int64) error {
	var done int64
	if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", "activity77.period.consumer", id).Count(&done).Error; err != nil {
		return err
	}
	if done > 0 {
		return nil
	}
	var live db.PeriodInstance
	if err := tx.First(&live, "id=?", id).Error; err != nil {
		return err
	}
	oldTiers, err := s.activity77PeriodTiers(pair.old, id, false)
	if err != nil {
		return err
	}
	definition, ok := adminActivityDefinitionFor(pair.old.Mode)
	if !ok || live.Kind != definition.kind || live.Source != pair.old.Source || live.Title != pair.old.Name || live.StartAt != start || live.EndAt != end || live.RewardEndAt != claim || !reflect.DeepEqual(p1PeriodMilestones(live), oldTiers) || live.RankPlaces != 0 {
		return fmt.Errorf("期次规则不匹配: %s", id)
	}
	nextTiers, err := s.activity77PeriodTiers(pair.next, id, true)
	if err != nil {
		return err
	}
	if len(nextTiers) == 0 || len(nextTiers) > 5 {
		return constUtil.ErrAdminConfiguration
	}
	next := live
	next.Goal = nextTiers[0].Goal
	next.Rewards = nextTiers[0].Rewards
	next.Milestones = adminJSON(nextTiers)
	next.Frozen = nil
	if err = s.validateP1Period(next); err != nil {
		return err
	}
	if len(live.Frozen) > 0 {
		var frozen db.PeriodInstance
		if json.Unmarshal(live.Frozen, &frozen) != nil || !activity77PeriodEqual(frozen, live) {
			return fmt.Errorf("期次冻结规则不匹配: %s", id)
		}
		frozen.Goal = next.Goal
		frozen.Rewards = next.Rewards
		frozen.Milestones = next.Milestones
		next.Frozen = adminJSON(frozen)
	}
	oldSources := p1PeriodSources(live)
	newSources := p1PeriodSources(next)
	var ids []int64
	if err = tx.Model(&db.PeriodProgress{}).Where("instance_id=?", id).Distinct("player_id").Limit(int(constUtil.Activity77RestaurantStateLimit)+1).Pluck("player_id", &ids).Error; err != nil {
		return err
	}
	var deliveryIDs []int64
	if err = tx.Model(&db.RewardDelivery{}).Where("source_key IN ?", oldSources).Distinct("player_id").Limit(int(constUtil.Activity77RestaurantStateLimit)+1).Pluck("player_id", &deliveryIDs).Error; err != nil {
		return err
	}
	owners := map[int64]bool{}
	for _, v := range ids {
		owners[v] = true
	}
	for _, v := range deliveryIDs {
		owners[v] = true
	}
	if len(owners) > int(constUtil.Activity77RestaurantStateLimit) {
		return fmt.Errorf("期次玩家数超过上限")
	}
	// Maintenance prevents new gameplay while each profile protects its progress and receipts.
	ordered := make([]int64, 0, len(owners))
	for v := range owners {
		ordered = append(ordered, v)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for _, playerID := range ordered {
		var profile db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&profile, playerID).Error; err != nil {
			return err
		}
	}
	var locked db.PeriodInstance
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", id).Error; err != nil {
		return err
	}
	if !reflect.DeepEqual(locked, live) {
		return constUtil.ErrAdminConfigConflict
	}
	var progressCount, retiredCount, mappedCount int
	now := time.Now().UnixMilli()
	for _, playerID := range ordered {
		var oldFirstID, newFirstID int64
		hasProgress := false
		var progress db.PeriodProgress
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id=? AND player_id=?", id, playerID).First(&progress).Error
		if err == nil {
			hasProgress = true
			var frozen db.PeriodInstance
			if json.Unmarshal(progress.Snapshot, &frozen) != nil || !activity77PeriodEqual(frozen, live) {
				return fmt.Errorf("玩家活动快照不匹配 %d", playerID)
			}
			frozen.Goal = next.Goal
			frozen.Rewards = next.Rewards
			frozen.Milestones = next.Milestones
			progressCount++
			if err = tx.Model(&progress).Update("snapshot", adminJSON(frozen)).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		for i, source := range oldSources {
			var receipt db.RewardDelivery
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND source_key=?", playerID, source).First(&receipt).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if i == 0 {
				oldFirstID = receipt.ID
			}
			if receipt.Rewards != oldTiers[i].Rewards {
				return fmt.Errorf("历史奖励内容不匹配 player=%d source=%s", playerID, source)
			}
			if receipt.ClaimedAt == 0 {
				if receipt.ExpiresAt == 0 || receipt.ExpiresAt > now {
					retiredCount++
					if err = tx.Model(&receipt).Update("expires_at", now).Error; err != nil {
						return err
					}
				}
				continue
			}
			for _, index := range activity77PeriodCovered(pair, i) {
				record := db.RewardDelivery{PlayerID: playerID, SourceKey: newSources[index], Title: next.Title, Content: "活动历史领取迁移: " + source, Rewards: nextTiers[index].Rewards, CreatedAt: now, ExpiresAt: next.RewardEndAt, ClaimedAt: receipt.ClaimedAt}
				mappedCount++
				if err = tx.Create(&record).Error; err != nil {
					return err
				}
				if index == 0 {
					newFirstID = record.ID
				}
			}
		}
		if hasProgress {
			if progress.RewardID != 0 && progress.RewardID != oldFirstID {
				return fmt.Errorf("活动进度奖励引用不匹配 player=%d", playerID)
			}
			if progress.RewardID != newFirstID {
				if err = tx.Model(&progress).Update("reward_id", newFirstID).Error; err != nil {
					return err
				}
			}
		}
	}
	if err = tx.Save(&next).Error; err != nil {
		return err
	}
	return adminAudit(tx, actor, "activity77.period.consumer", id, fmt.Sprintf("迁移进度=%d 过期旧未领=%d 历史已领覆盖=%d", progressCount, retiredCount, mappedCount))
}

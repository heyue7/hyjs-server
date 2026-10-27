package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ApplyActivity77WelfareKing is an explicit maintenance operation; startup never calls it.
func (s *GameService) ApplyActivity77WelfareKing(ctx context.Context, actor *model.AdminPrincipal) error {
	var policy db.ClientUpdatePolicy
	if err := db.GetIns().WithContext(ctx).First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Maintenance {
		return fmt.Errorf("先关服再应用餐厅与王者排期修复")
	}
	if err := s.activity77Restaurant(ctx, actor); err != nil {
		return fmt.Errorf("餐厅修复: %w", err)
	}
	return s.activity77King(ctx, actor)
}

func activity77SameMeal(a, b db.P4WelfareRule) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.Source == b.Source && a.StartAt == b.StartAt && a.EndAt == b.EndAt && a.ClaimEndAt == b.ClaimEndAt && bytes.Equal(a.Rules, b.Rules)
}

func (s *GameService) activity77Restaurant(ctx context.Context, actor *model.AdminPrincipal) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		server := s.OriginalServerID()
		guard, err := adminActivityScheduleRevision(tx, server, true)
		if err != nil {
			return err
		}
		id := "restore_re1051_meal_" + strconv.FormatInt(server, 10)
		var occurrence db.AdminActivityOccurrence
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&occurrence, "id=? AND server_id=?", id, server).Error; err != nil {
			return err
		}
		key := "p4:meal:admin_re1051_schedule_" + strconv.FormatInt(server, 10)
		if occurrence.Mode != "stamina-meal" || occurrence.ConsumerKey != key || occurrence.ConsumerID != "welfare:"+key {
			return constUtil.ErrAdminConfiguration
		}
		var rule db.P4WelfareRule
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rule, "id=?", key).Error; err != nil {
			return err
		}
		if rule.Kind != constUtil.P4WelfareMeal || rule.Source != constUtil.ReRestaurantSource || rule.StartAt != occurrence.StartAt || rule.EndAt != occurrence.EndAt || rule.ClaimEndAt != occurrence.ClaimEndAt {
			return constUtil.ErrAdminConfiguration
		}
		var rules p4WelfareRules
		if json.Unmarshal(rule.Rules, &rules) != nil || len(rules.Meals) != 2 || rules.Meals[0] != (p4Meal{660, 810, 180}) || rules.Meals[1] != (p4Meal{1020, 1170, 180}) {
			return constUtil.ErrAdminConfiguration
		}
		var form model.AdminActivityForm
		if json.Unmarshal(occurrence.FormJSON, &form) != nil || form.StartAt != rule.StartAt || form.EndAt != rule.EndAt || form.ClaimEndAt != rule.ClaimEndAt {
			return constUtil.ErrAdminConfiguration
		}
		var conflict int64
		if err = tx.Model(&db.AdminActivityOccurrence{}).Where("server_id=? AND mode=? AND id<>? AND enabled=? AND archived=? AND claim_end_at>?", server, "stamina-meal", id, true, false, time.Now().UnixMilli()).Count(&conflict).Error; err != nil {
			return err
		}
		if conflict != 0 {
			return fmt.Errorf("其他餐厅排期冲突")
		}
		var applied int64
		if err = tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", "activity77.restaurant", strconv.FormatInt(server, 10)).Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil
		}
		zone := time.FixedZone("UTC+8", 8*3600)
		now := time.Now().In(zone)
		end := time.Date(now.Year()+10, now.Month(), now.Day(), 0, 0, 0, 0, zone).UnixMilli()
		if end <= rule.EndAt {
			return fmt.Errorf("餐厅原结束日期超过修复目标")
		}
		original := rule
		var frozen db.P4WelfareRule
		if len(rule.Frozen) > 0 {
			if json.Unmarshal(rule.Frozen, &frozen) != nil || !activity77SameMeal(frozen, original) {
				return fmt.Errorf("餐厅冻结规则不匹配")
			}
			frozen.EndAt = end
			frozen.ClaimEndAt = end
			rule.Frozen = adminJSON(frozen)
		}
		var count int64
		if err = tx.Model(&db.P4WelfareState{}).Where("rule_id=?", key).Count(&count).Error; err != nil {
			return err
		}
		if count > constUtil.Activity77RestaurantStateLimit {
			return fmt.Errorf("餐厅领取记录超过单次修复上限: %d", count)
		}
		var lastID int64
		for {
			var ids []int64
			if err = tx.Model(&db.P4WelfareState{}).Where("rule_id=? AND player_id>?", key, lastID).Order("player_id").Limit(constUtil.Activity77RestaurantBatchSize).Pluck("player_id", &ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				break
			}
			for _, playerID := range ids {
				var profile db.Profile
				if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&profile, playerID).Error; err != nil {
					return err
				}
				var state db.P4WelfareState
				if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "player_id=? AND rule_id=?", playerID, key).Error; err != nil {
					return err
				}
				var snapshot db.P4WelfareRule
				if json.Unmarshal(state.Snapshot, &snapshot) != nil || !activity77SameMeal(snapshot, original) {
					return fmt.Errorf("餐厅玩家冻结快照不匹配 player=%d", playerID)
				}
				snapshot.EndAt = end
				snapshot.ClaimEndAt = end
				result := tx.Model(&db.P4WelfareState{}).Where("player_id=? AND rule_id=? AND snapshot=?", playerID, key, state.Snapshot).Update("snapshot", adminJSON(snapshot))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return constUtil.ErrAdminConfigConflict
				}
				lastID = playerID
			}
		}

		rule.EndAt = end
		rule.ClaimEndAt = end
		rule.Enabled = true
		if err = tx.Save(&rule).Error; err != nil {
			return err
		}
		form.EndAt = end
		form.ClaimEndAt = end
		result := tx.Model(&db.AdminActivityOccurrence{}).Where("id=? AND revision=?", id, occurrence.Revision).Updates(map[string]any{"end_at": end, "claim_end_at": end, "enabled": true, "archived": false, "form_json": adminJSON(form), "revision": occurrence.Revision + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return constUtil.ErrAdminConfigConflict
		}
		guard.Revision++
		if err = tx.Save(&guard).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, "activity77.restaurant", strconv.FormatInt(server, 10), "沿用原规则和领取记录，延长餐厅有效期")
	})
}

func (s *GameService) activity77King(ctx context.Context, actor *model.AdminPrincipal) error {
	tx := db.GetIns().WithContext(ctx)
	var realm db.AdminGameServer
	if err := tx.First(&realm, s.OriginalServerID()).Error; err != nil {
		return err
	}
	if realm.OpenAt <= 0 {
		return constUtil.ErrAdminConfiguration
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	opened := time.UnixMilli(realm.OpenAt).In(zone)
	start := time.Date(opened.Year(), opened.Month(), opened.Day(), 0, 0, 0, 0, zone)
	var pool db.GameplayPool
	if err := tx.First(&pool, "id=?", "recruit_3").Error; err != nil {
		return err
	}
	var prizes []db.GameplayPoolReward
	if err := tx.Where("pool_id=?", "recruit_3").Order("id").Find(&prizes).Error; err != nil {
		return err
	}
	base, err := s.adminKingRecruitActivityForm(tx, pool, prizes)
	if err != nil {
		return err
	}
	defs, err := s.AdminActivityDefinitions(ctx, actor)
	if err != nil {
		return err
	}
	templates := make(map[int64]*model.AdminActivityDefinitionDTO)
	for world := int64(1); world <= 3; world++ {
		form := base
		form.StartAt = 0
		form.EndAt = 0
		form.ClaimEndAt = 0
		form.Options.KingWorld = world
		form.Name = fmt.Sprintf("王者招募 %s", map[int64]string{1: "SAO", 2: "ALO", 3: "GGO"}[world])
		form.Source = fmt.Sprintf("成长投放V77 · 王者世界%d", world)
		form.Alias = "成长投放V77"
		form.Prizes = nil
		for _, prize := range base.Prizes {
			hero := s.table("Hero", "id", prize.ResourceID)
			if hero == nil {
				return constUtil.ErrAdminConfiguration
			}
			if hero["quality"] == "23" && hero["space"] != strconv.FormatInt(world, 10) {
				continue
			}
			form.Prizes = append(form.Prizes, prize)
		}
		for i := range defs {
			if defs[i].Form.Source == form.Source {
				if templates[world] != nil {
					return fmt.Errorf("王者模板重复: %s", form.Source)
				}
				candidate := defs[i]
				if candidate.Form.Options.KingWorld != world || candidate.Name != adminActivityCanonicalForm(form).Name || !candidate.Enabled || !bytes.Equal(adminJSON(candidate.Form.Prizes), adminJSON(form.Prizes)) {
					return fmt.Errorf("王者模板与预期不符: %s", form.Source)
				}
				templates[world] = &candidate
			}
		}
		if templates[world] == nil {
			saved, e := s.AdminSaveActivityDefinition(ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 3, Enabled: true})
			if e != nil {
				return e
			}
			templates[world] = saved
		}
	}
	calendar, err := s.AdminActivitySchedule(ctx, actor, s.OriginalServerID())
	if err != nil {
		return err
	}
	items := calendar.Items
	changed := false
	for i := range items {
		if items[i].Mode == constUtil.AdminKingRecruitMode && growth74Source(items[i].Form.Source) && items[i].Enabled {
			items[i].Enabled = false
			changed = true
		}
	}
	type slot struct {
		world int64
		day   int
	}
	slots := []slot{{1, 0}, {2, 0}, {3, 0}}
	for day := 3; day < 183; day += 3 {
		slots = append(slots, slot{int64((day-3)/3%3 + 1), day})
	}
	for _, slot := range slots {
		at := start.AddDate(0, 0, slot.day).UnixMilli()
		end := start.AddDate(0, 0, slot.day+3).UnixMilli()
		def := templates[slot.world]
		found := false
		for _, existing := range items {
			if existing.TemplateID == def.ID && existing.StartAt == at && existing.EndAt == end {
				if !existing.Enabled {
					return fmt.Errorf("王者目标排期已被停用，需人工核查")
				}
				found = true
				break
			}
		}
		if !found {
			changed = true
			items = append(items, model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Name: def.Name, Mode: def.Mode, Timing: "calendar", StartAt: at, EndAt: end, ClaimEndAt: end, Lane: 16, Enabled: true})
		}
	}
	if !changed {
		return nil
	}
	preview, err := s.AdminPreviewActivitySchedule(ctx, actor, s.OriginalServerID(), model.AdminActivitySchedulePreviewRequest{Version: calendar.Version, Items: items})
	if err != nil {
		return err
	}
	if !preview.Valid {
		return fmt.Errorf("王者排期冲突: %+v", preview.Conflicts)
	}
	_, err = s.AdminSaveActivitySchedule(ctx, actor, s.OriginalServerID(), model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: items})
	return err
}

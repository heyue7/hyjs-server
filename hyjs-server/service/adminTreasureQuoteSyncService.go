package service

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// adminLockTreasureCalendars follows the calendar -> template -> consumer lock order.
// Template edits are infrequent global operations; game requests never acquire these rows.
func adminLockTreasureCalendars(tx *gorm.DB) error {
	var rows []db.AdminActivityScheduleRevision
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("server_id").Find(&rows).Error
}

// adminSyncTreasureTemplate updates future quotes, retaining each paid receipt and role's last quote.
// Dates, activation, ownership and historical/archived occurrences remain under their own calendars.
func (s *GameService) adminSyncTreasureTemplate(tx *gorm.DB, definition db.AdminActivityDefinition, form model.AdminActivityForm, now time.Time) error {
	var (
		occurrences []db.AdminActivityOccurrence
		rules       = reOperationsWelfareRules{Tiers: []reOperationsWelfareTier{}}
		servers     = map[int64]bool{}
	)
	for _, milestone := range form.Milestones {
		reward, err := s.adminTypedResourceSpec(milestone.Rewards, false)
		if err != nil || len(milestone.Rewards) != 1 || milestone.Rewards[0].ResourceID != "coin" {
			return constUtil.ErrAdminInput
		}
		rules.Tiers = append(rules.Tiers, reOperationsWelfareTier{Cost: milestone.Goal, Reward: reward})
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("template_id=? AND mode=? AND archived=? AND claim_end_at>?", definition.ID, "fortune-cat", false, now.UnixMilli()).Order("server_id,id").Find(&occurrences).Error; err != nil {
		return err
	}
	for _, occurrence := range occurrences {
		domain, key, ok := strings.Cut(occurrence.ConsumerID, ":")
		// Personal/opening schedules instantiate their own consumer; their frozen scope remains separate.
		if !ok || domain != "welfare" {
			continue
		}
		var (
			cfg      db.P4WelfareRule
			retained model.AdminActivityForm
		)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, "id=? AND kind=?", key, constUtil.ReWelfareCatKind).Error; err != nil {
			return err
		}
		if json.Unmarshal(occurrence.FormJSON, &retained) != nil {
			return constUtil.ErrAdminConfiguration
		}
		cfg.Rules = adminJSON(rules)
		if err := s.validateReOperationsWelfare(cfg); err != nil {
			return err
		}
		// Frozen is historical evidence. Info now explicitly binds the live quote; claim keeps its last Info snapshot.
		if err := tx.Model(&cfg).Update("rules", cfg.Rules).Error; err != nil {
			return err
		}
		retained.Milestones = form.Milestones
		if err := tx.Model(&occurrence).Updates(map[string]any{"form_json": adminJSON(retained), "template_version": adminVersion(definition), "revision": occurrence.Revision + 1}).Error; err != nil {
			return err
		}
		servers[occurrence.ServerID] = true
	}
	for serverID := range servers {
		if err := tx.Model(&db.AdminActivityScheduleRevision{}).Where("server_id=?", serverID).Update("revision", gorm.Expr("revision+1")).Error; err != nil {
			return err
		}
	}
	return nil
}

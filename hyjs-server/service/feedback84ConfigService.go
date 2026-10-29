package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ApplyFeedback84Config explicitly publishes sweeping for the audited original
// permanent policy. Historical offers, matches and settlements remain frozen.
func (s *GameService) ApplyFeedback84Config(ctx context.Context, actor *model.AdminPrincipal) error {
	if s.OriginalServerID() != 1 || !s.ServerClockReady {
		return constUtil.ErrAdminConfiguration
	}
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	ctx, _, err := s.bindServerGameClockWorker75(ctx)
	if err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var clock db.ServerGameClock
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&clock, "server_id=?", s.OriginalServerID()).Error; err != nil {
			return err
		}
		if !serverGameClockValid(&clock) {
			return constUtil.ErrAdminConfiguration
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var operator db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&operator, actor.UserID).Error; err != nil || !operator.Enabled || operator.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		var maintenance db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&maintenance, 1).Error; err != nil {
			return err
		}
		if !maintenance.Maintenance {
			return fmt.Errorf("%w: 须在维护期执行", constUtil.ErrAdminConfigConflict)
		}
		var cfg db.P11PlunderFightPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
			return err
		}
		var applied db.AdminAudit
		err := tx.Where("action=? AND target=?", constUtil.Feedback84Migration, constUtil.Feedback84PolicyTarget).Order("id DESC").First(&applied).Error
		if err == nil {
			var detail struct {
				After db.P11PlunderFightPolicy `json:"after"`
			}
			if json.Unmarshal([]byte(applied.Detail), &detail) != nil || !reflect.DeepEqual(detail.After, cfg) {
				return constUtil.ErrAdminConfigConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var defaults []db.AdminAudit
		if err := tx.Where("target=?", constUtil.Feedback84PolicyTarget).Order("id").Limit(3).Find(&defaults).Error; err != nil {
			return err
		}
		var detail struct {
			After   db.P11PlunderFightPolicy `json:"after"`
			Source  string                   `json:"source"`
			Applied bool                     `json:"applied"`
		}
		if len(defaults) != 1 || defaults[0].Action != constUtil.P11PlunderPermanentMigration || defaults[0].Actor != "system" || json.Unmarshal([]byte(defaults[0].Detail), &detail) != nil || !detail.Applied || detail.Source != constUtil.P11PlunderPermanentSource || !reflect.DeepEqual(detail.After, cfg) {
			return fmt.Errorf("%w: 夺宝策略已偏离系统常驻配置", constUtil.ErrAdminConfigConflict)
		}
		if !cfg.Enabled || !cfg.Permanent || cfg.SweepEnabled || cfg.Version != constUtil.P11PlunderPermanentVersion || cfg.Source != constUtil.P11PlunderFightPolicySource || cfg.EndAt != 0 || cfg.PublishedAt < 1 || cfg.StartAt != cfg.PublishedAt || cfg.MatchCount != 3 || cfg.ScanLimit != 512 || cfg.LevelDifference != 20 || cfg.MatchMillis != 300000 || cfg.ChanceTier != 3 || cfg.DropPPM != 250000 {
			return constUtil.ErrAdminConfigConflict
		}
		before := cfg
		cfg.Version, cfg.SweepEnabled = constUtil.Feedback84PlunderVersion, true
		if err := tx.Model(&cfg).Updates(map[string]any{"version": cfg.Version, "sweep_enabled": true}).Error; err != nil {
			return err
		}
		return tx.Create(&db.AdminAudit{Time: time.Now().UnixMilli(), Actor: actor.Username, Action: constUtil.Feedback84Migration, Target: constUtil.Feedback84PolicyTarget, Detail: string(adminJSON(map[string]any{"before": before, "after": cfg}))}).Error
	})
}

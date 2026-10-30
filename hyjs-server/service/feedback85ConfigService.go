package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ApplyFeedback85Config is explicit and atomic. Startup never reopens an
// administrator's changed configuration or rewrites already accepted runs.
func (s *GameService) ApplyFeedback85Config(ctx context.Context, actor *model.AdminPrincipal) error {
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
		var applied db.AdminAudit
		appliedErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("action=? AND target=?", constUtil.Awake85Migration, constUtil.Awake85Target).Order("id DESC").First(&applied).Error
		if appliedErr != nil && !errors.Is(appliedErr, gorm.ErrRecordNotFound) {
			return appliedErr
		}
		published := appliedErr == nil
		if published {
			for _, expected := range training85Defaults() {
				var row db.WarriorGrowthRule
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", expected.ID).Error; err != nil {
					return err
				}
				a, _ := json.Marshal(row)
				b, _ := json.Marshal(expected)
				if string(a) != string(b) {
					return constUtil.ErrAdminConfigConflict
				}
			}
		}
		if err := applyWarriorTraining85(tx); err != nil {
			return err
		}
		var policies []db.P7AwakePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", []int64{constUtil.P7AwakeExplorePolicyID, constUtil.P7AwakeBattlePolicyID}).Order("id").Find(&policies).Error; err != nil {
			return err
		}
		if len(policies) != 2 {
			return constUtil.ErrAdminConfigConflict
		}
		before := append([]db.P7AwakePolicy(nil), policies...)
		for _, policy := range policies {
			expectedOld, expectedNew := constUtil.P7AwakeExploreSource, constUtil.Awake85ExploreSource
			if policy.ID == constUtil.P7AwakeBattlePolicyID {
				expectedOld, expectedNew = constUtil.P7AwakeBattleSource, constUtil.Awake85BattleSource
			}
			if policy.MinLevel != 70 || (policy.Source != expectedOld && policy.Source != expectedNew) {
				return constUtil.ErrAdminConfigConflict
			}
			if !published && policy.Source != expectedOld {
				return constUtil.ErrAdminConfigConflict
			}
			if published && (policy.Source != expectedNew || !policy.Enabled) {
				return constUtil.ErrAdminConfigConflict
			}
			if policy.Source == expectedNew && !policy.Enabled {
				return constUtil.ErrAdminConfigConflict
			}
			if policy.Source != expectedNew || !policy.Enabled {
				if err := tx.Model(&policy).Updates(map[string]any{"enabled": true, "source": expectedNew}).Error; err != nil {
					return err
				}
			}
		}
		var legacy db.P7AwakeStageRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&legacy, "copy_id=?", constUtil.P7AwakeExploreCopy).Error; err != nil {
			return err
		}
		expectedLegacy, err := p7AwakeLocalCatalogue(legacy.Enabled)
		if err != nil {
			return err
		}
		if !p7AwakeRuleEqual85(legacy, expectedLegacy) {
			return constUtil.ErrAdminConfigConflict
		}
		if published && legacy.Enabled {
			return constUtil.ErrAdminConfigConflict
		}
		if legacy.Enabled {
			if err := tx.Model(&legacy).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		for _, chapter := range awake85Chapters() {
			var row db.P7AwakeStageRule
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "copy_id=?", chapter.copyID).Error
			if published && errors.Is(err, gorm.ErrRecordNotFound) {
				return constUtil.ErrAdminConfigConflict
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				row, err = awake85Catalogue(chapter, true)
				if err != nil {
					return err
				}
				if err = tx.Create(&row).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if !published && row.Enabled {
				return constUtil.ErrAdminConfigConflict
			}
			if published && !row.Enabled {
				return constUtil.ErrAdminConfigConflict
			}
			expected, err := awake85Catalogue(chapter, row.Enabled)
			if err != nil {
				return err
			}
			if !p7AwakeRuleEqual85(row, expected) {
				return constUtil.ErrAdminConfigConflict
			}
			if !row.Enabled {
				if err := tx.Model(&row).Update("enabled", true).Error; err != nil {
					return err
				}
			}
		}
		if published {
			return nil
		}
		return tx.Create(&db.AdminAudit{Time: time.Now().UnixMilli(), Actor: actor.Username, Action: constUtil.Awake85Migration, Target: constUtil.Awake85Target, Detail: string(adminJSON(map[string]any{"before": before, "source": constUtil.Awake85ExploreSource}))}).Error
	})
}

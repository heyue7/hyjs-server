package service

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func (s *GameService) p9RecruitDefaultPool80() ([]p9RecruitOption, error) {
	pool := []p9RecruitOption{}
	purple, orange := 0, 0
	for _, pet := range s.Tables["Pet"] {
		switch pet["quality"] {
		case "18":
			purple++
			pool = append(pool, p9RecruitOption{PetID: pet["id"], Weight: 3762})
		case "20":
			orange++
			pool = append(pool, p9RecruitOption{PetID: pet["id"], Weight: 21, Guarantee: true})
		}
	}
	if purple != 21 || orange != 38 {
		return nil, errors.New(constUtil.P9RecruitConfigurationError)
	}
	return pool, nil
}

// ApplyFeedback80Config changes the live rule once, without scanning player saves.
func (s *GameService) ApplyFeedback80Config(ctx context.Context, actor *model.AdminPrincipal) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var maintenance db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&maintenance, 1).Error; err != nil {
			return err
		}
		if !maintenance.Maintenance {
			return constUtil.ErrAdminConfigConflict
		}
		var applied int64
		if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.P9Recruit80Audit, "ordinary").Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil
		}
		var rule db.P9PetRecruitRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rule, "id=?", constUtil.P9RecruitRuleID).Error; err != nil {
			return err
		}
		if rule.Source != constUtil.P9RecruitLegacySource && rule.Source != constUtil.P9RecruitSource {
			return constUtil.ErrAdminConfigConflict
		}
		var old p9RecruitInput
		oldInput := rule
		// Validate the existing legacy rule before replacing its pool.
		if json.Unmarshal(oldInput.Pool, &old.Pool) != nil || len(old.Pool) == 0 {
			return constUtil.ErrAdminConfigConflict
		}
		pool, err := s.p9RecruitDefaultPool80()
		if err != nil {
			return err
		}
		raw, err := json.Marshal(pool)
		if err != nil {
			return err
		}
		candidate := rule
		candidate.Pool, candidate.Source = raw, constUtil.P9RecruitSource
		validated, err := s.p9RecruitInput(candidate)
		if err != nil {
			return err
		}
		validated.Pool, validated.Source = old.Pool, rule.Source
		if err = s.validateP9RecruitInput(validated); err != nil {
			return err
		}
		if err := tx.Model(&rule).Updates(map[string]any{"pool": raw, "source": constUtil.P9RecruitSource}).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, constUtil.P9Recruit80Audit, "ordinary", "精灵普通扭蛋：紫99%、橙1%、红0；原十抽保底、免费、库存和历史回执保持")
	})
}

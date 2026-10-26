package service

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func copyDropsDTO(r db.CopySpecialRule) *model.AdminCopyDropsResponse {
	return &model.AdminCopyDropsResponse{Enabled: r.Enabled, Cycle: r.Cycle, Version: adminVersion(r), Source: constUtil.CopyFragmentSource}
}

// AdminCopyDrops reads the fragment-only local policy.
func (s *GameService) AdminCopyDrops(ctx context.Context, a *model.AdminPrincipal) (*model.AdminCopyDropsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, a, false); err != nil {
		return nil, err
	}
	var row db.CopySpecialRule
	if err := tx.First(&row, "id=?", constUtil.CopyFragmentRuleID).Error; err != nil {
		return nil, err
	}
	return copyDropsDTO(row), nil
}

// AdminSaveCopyDrops preserves operator disable choices across initialization.
func (s *GameService) AdminSaveCopyDrops(ctx context.Context, a *model.AdminPrincipal, r model.AdminCopyDropsSaveRequest) (*model.AdminCopyDropsResponse, error) {
	var out *model.AdminCopyDropsResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, a, true); err != nil {
			return err
		}
		var row db.CopySpecialRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", constUtil.CopyFragmentRuleID).Error; err != nil {
			return err
		}
		if r.Version != adminVersion(row) {
			return constUtil.ErrAdminConfigConflict
		}
		row.Enabled, row.Cycle, row.Source = r.Enabled, r.Cycle, constUtil.CopyFragmentOperatorSource
		row.Revision++
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, a, "copy.fragments.update", "global", r.Reason); err != nil {
			return err
		}
		out = copyDropsDTO(row)
		return nil
	})
	return out, err
}

package service

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

// AdminDeleteActivityDefinition hides the global template without rewriting issued gameplay contracts.
// The archived primary key is retained so startup import cannot recreate an operator-deleted template.
func (s *GameService) AdminDeleteActivityDefinition(ctx context.Context, p *model.AdminPrincipal, id, version string) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var row db.AdminActivityDefinition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; err != nil {
			return err
		}
		if adminVersion(row) != version {
			return constUtil.ErrAdminConfigConflict
		}
		row.Archived = true
		row.Enabled = false
		row.Revision++
		row.UpdatedAtMillis = time.Now().UnixMilli()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.template.archive", row.ID, "已删除全局模板；已发排期和玩家领取历史保持")
	})
}

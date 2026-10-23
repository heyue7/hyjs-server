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

// InitializeAdminSproutSettings 创建全局小芽设置；未指定天数时保持关闭。
func (s *GameService) InitializeAdminSproutSettings(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.AdminSproutSetting{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.AdminSproutSetting{ID: 1, Revision: 1}).Error
}
func adminSproutDTO(row db.AdminSproutSetting) *model.AdminSproutSettingsResponse {
	return &model.AdminSproutSettingsResponse{Enabled: row.Enabled, Days: row.Days, Icon: constUtil.AdminSproutIcon, Version: adminVersion(row)}
}

// AdminSproutSettings 读取与区服、地图无关的全局配置。
func (s *GameService) AdminSproutSettings(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminSproutSettingsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	var row db.AdminSproutSetting
	if err := tx.First(&row, 1).Error; err != nil {
		return nil, err
	}
	return adminSproutDTO(row), nil
}

// AdminSaveSproutSettings 在单事务中检查版本、保存并审计。
func (s *GameService) AdminSaveSproutSettings(ctx context.Context, actor *model.AdminPrincipal, req model.AdminSproutSettingsSaveRequest) (*model.AdminSproutSettingsResponse, error) {
	var out *model.AdminSproutSettingsResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var row db.AdminSproutSetting
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
			return err
		}
		if req.Version != adminVersion(row) {
			return constUtil.ErrAdminConfigConflict
		}
		row.Enabled, row.Days = req.Enabled, req.Days
		row.Revision++
		row.UpdatedAt, row.UpdatedBy = time.Now().UnixMilli(), actor.Username
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, actor, "sprout.settings.update", "global", req.Reason); err != nil {
			return err
		}
		out = adminSproutDTO(row)
		return nil
	})
	return out, err
}

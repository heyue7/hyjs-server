package service

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
)

// initializeDesertFieldName 仅迁移后台沙漠模板的旧默认名称；保留自定义名称与部署快照。
func (s *GameService) initializeDesertFieldName(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []db.AdminFieldTemplate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("scene=? AND name IN ?", "field_003", []string{"野外三区", "野外三区 · 原资源方案"}).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			name := "沙漠野外"
			if row.Name == "野外三区 · 原资源方案" {
				name += " · 原资源方案"
			}
			if err := tx.Model(&db.AdminFieldTemplate{}).Where("id=?", row.ID).Updates(map[string]any{"name": name, "revision": gorm.Expr("revision + 1"), "updated_at": time.Now().UnixMilli()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

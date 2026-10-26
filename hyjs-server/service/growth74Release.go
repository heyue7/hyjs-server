package service

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

// New original-format item rows require the companion APK before reopening.
// No announcement, reminder batch or automatic reopening is triggered.
func (s *GameService) growth74Release(ctx context.Context, a *model.AdminPrincipal) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p db.ClientUpdatePolicy
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, 1).Error; e != nil {
			return e
		}
		if p.LatestBuild >= 2026102604 && p.MinLoginBuild >= 2026102604 {
			return nil
		}
		now := time.Now().UnixMilli()
		r := db.ClientUpdateRelease{BuildCode: 2026102604, VersionName: "1.0.74", Title: "成长投放测试版本", Content: "请安装1.0.74测试包后进入游戏。", Mode: "maintenance", MinLoginBuild: 2026102604, MinRunBuild: 2026102604, MaintenanceStartAt: p.MaintenanceStartAt, State: "published", PublishedAt: now, UpdatedAt: now, PublishedBy: a.Username}
		if e := tx.Create(&r).Error; e != nil {
			return e
		}
		p.ReleaseID = r.ID
		p.LatestBuild = r.BuildCode
		p.VersionName = r.VersionName
		p.Title = r.Title
		p.Content = r.Content
		p.Mode = r.Mode
		p.MinLoginBuild = r.MinLoginBuild
		p.MinRunBuild = r.MinRunBuild
		p.LoginGateAt = now
		p.NoticeEnabled = false
		p.Revision++
		if e := tx.Save(&p).Error; e != nil {
			return e
		}
		return adminAudit(tx, a, "growth74.client.compatibility", "1.0.74", "新道具表与余额图标需要74客户端；维护保持，不发送公告")
	})
}

package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ApplyVIP10BookRestore runs the one-time maintenance operation without starting
// listeners or running unrelated startup migrations.
func (s *GameService) ApplyVIP10BookRestore(ctx context.Context) error {
	var user db.AdminUser
	if err := db.GetIns().WithContext(ctx).Where("enabled=? AND role=?", true, constUtil.AdminRoleAdmin).Order("id").First(&user).Error; err != nil {
		return err
	}
	return s.RestoreVIP10BookChannel(ctx, &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role})
}

// RestoreVIP10BookChannel enables the reviewed stable product during explicit
// maintenance. It changes no player asset, payment qualification or historical claim.
// A committed audit is the durable one-shot marker; retries preserve later operator edits.
func (s *GameService) RestoreVIP10BookChannel(ctx context.Context, actor *model.AdminPrincipal) error {
	if s.OriginalServerID() != 1 {
		return constUtil.ErrAdminConfiguration
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var user db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, actor.UserID).Error; err != nil {
			return err
		}
		if !user.Enabled || user.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		// The existing singleton serializes deployments without a new state table.
		var policy db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Maintenance {
			return constUtil.ErrAdminConfigConflict
		}
		var applied int64
		if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=? AND detail=?", constUtil.VIPBookRestoreAction, constUtil.P3VIP10GrowthProduct, constUtil.VIPBookRestoreDetail).Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil
		}
		var product db.VIPShopProduct
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, "id=?", constUtil.P3VIP10GrowthProduct).Error; err != nil {
			return err
		}
		if product.Enabled || product.MinVIP != 10 || product.Cost != 1100 || product.CostResource != "diamond" || product.TotalLimit != 0 || product.Reward != "item_044,1" || product.Source != constUtil.P3VIP10GrowthSource {
			return fmt.Errorf("VIP10商品已偏离审核快照，保留现有运营配置：%w", constUtil.ErrAdminConfigConflict)
		}
		matches := 0
		for _, row := range s.Tables["ItemUse"] {
			if row["itemId"] != "item_044" {
				continue
			}
			matches++
			if row["category"] != "2" || row["odds"] != "100" || row["result"] != constUtil.VIPBookRestoreBoxResult {
				return constUtil.ErrAdminConfiguration
			}
		}
		if matches != 1 {
			return constUtil.ErrAdminConfiguration
		}
		if err := tx.Model(&product).Update("enabled", true).Error; err != nil {
			return err
		}
		// Record the authenticated database identity in the same transaction.
		auditor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
		return adminAudit(tx, auditor, constUtil.VIPBookRestoreAction, product.ID, constUtil.VIPBookRestoreDetail)
	})
}

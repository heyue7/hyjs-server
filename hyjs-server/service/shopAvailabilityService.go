package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"strings"
	"time"
)

// InitializeAdminShopAvailability enables unlimited hours once for each shop.
// Existing catalogues, disabled products, frozen terms and purchase history stay intact.
func (s *GameService) InitializeAdminShopAvailability(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.AdminShopAvailability{}); err != nil {
		return err
	}
	for _, definition := range AdminShopDefinitions() {
		row := db.AdminShopAvailability{ShopKey: definition.ID, Unlimited: true, Revision: 1}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// AdminShopAvailability returns the shop-level hours; nil retains legacy isolated-database behavior.
func AdminShopAvailability(tx *gorm.DB, key string) (*db.AdminShopAvailability, error) {
	if !tx.Migrator().HasTable(&db.AdminShopAvailability{}) {
		return nil, nil
	}
	var row db.AdminShopAvailability
	err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&row, "shop_key=?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !row.Unlimited && (row.StartAt != 0 || row.EndAt != 0 || row.ClaimEndAt != 0) && (row.StartAt <= 0 || row.EndAt <= row.StartAt || row.ClaimEndAt < row.EndAt) {
		return nil, constUtil.ErrAdminConfiguration
	}
	return &row, nil
}

// requireShopAvailability changes operating hours only; native gameplay prerequisites still apply.
func (s *GameService) requireShopAvailability(tx *gorm.DB, key string, now time.Time) error {
	row, err := AdminShopAvailability(tx, key)
	if err != nil {
		return err
	}
	if row == nil || row.Unlimited {
		return nil
	}
	open := now.UnixMilli() >= row.StartAt && now.UnixMilli() < row.EndAt
	scheduledOpen, managed, err := s.AdminScheduledShopWindow(tx, key, now.UnixMilli())
	if err != nil {
		return err
	}
	if managed {
		open = scheduledOpen
	}
	if !open {
		return constUtil.ErrP4CommerceNotOpen
	}
	return nil
}

// p4ShopSourceWindow applies the shop-level hours without changing the source
// snapshot. Activity-native sources retain their individual schedule dates.
func p4ShopSourceWindow(tx *gorm.DB, cfg db.P4CommerceRule, now time.Time) (bool, error) {
	if strings.HasPrefix(cfg.ID, "shop_schedule_") || cfg.ShopType <= 0 || !AdminShopValid(cfg.Kind) {
		return now.UnixMilli() >= cfg.StartAt && now.UnixMilli() < cfg.EndAt, nil
	}
	row, err := AdminShopAvailability(tx, cfg.Kind)
	if err != nil {
		return false, err
	}
	if row == nil {
		return now.UnixMilli() >= cfg.StartAt && now.UnixMilli() < cfg.EndAt, nil
	}
	return row.Unlimited || now.UnixMilli() >= row.StartAt && now.UnixMilli() < row.EndAt, nil
}

// AdminOrdinaryShopSource reads the same latest catalogue managed in the shop workspace.
// Its frozen terms remain authoritative; scheduling creates a new consumer identity.
func (s *GameService) AdminOrdinaryShopSource(tx *gorm.DB, key string) (db.P4CommerceRule, error) {
	var source db.P4CommerceRule
	if !AdminShopValid(key) || key == "vip" || key == "pet-market" || key == "union-fight" {
		return source, constUtil.ErrAdminInput
	}
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("kind=? AND id NOT LIKE ?", key, "shop_schedule_%").Order("start_at DESC,id DESC").First(&source).Error; err != nil {
		return source, err
	}
	frozen, _, err := s.p4CommerceSnapshot(source)
	return frozen, err
}

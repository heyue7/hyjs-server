package service

import (
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func initializeGGOAccountWeekly(tx *gorm.DB) error {
	if tx.Dialector.Name() != "mysql" {
		return tx.AutoMigrate(&db.GGOAccountWeekly{})
	}
	var statement = strings.Replace(db.GGOAccountWeeklySchema, "CREATE TABLE IF NOT EXISTS "+constUtil.GGOAccountWeeklyTable, "CREATE TABLE IF NOT EXISTS "+db.AccountsTableName(constUtil.GGOAccountWeeklyTable), 1)
	return tx.Exec(statement).Error
}

// A real business week is shared across realms, independent of role-specific
// test calendar offsets and catalogue versions. Monday midnight starts a week.
func ggoAccountWeek(now time.Time) string {
	var local = now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	return local.AddDate(0, 0, -(int(local.Weekday())+6)%7).Format("2006-01-02")
}

func ggoWeeklyCatalogue(tx *gorm.DB, playerID int64, products []p4Product, now time.Time) (map[string]int64, int64, error) {
	var (
		ids    []string
		counts = map[string]int64{}
		owner  db.Profile
		rows   []db.GGOAccountWeekly
	)
	for _, product := range products {
		if product.AccountWeeklyLimit > 0 {
			ids = append(ids, ggoSourceProductID(product))
		}
	}
	if len(ids) == 0 {
		return counts, 0, nil
	}
	if err := tx.Select("id", "account_id").First(&owner, playerID).Error; err != nil {
		return nil, 0, err
	}
	if owner.AccountID < 1 {
		return counts, 0, nil
	}
	if err := db.AccountsTable(tx, constUtil.GGOAccountWeeklyTable).Where("account_id=? AND product_id IN ? AND week_start=?", owner.AccountID, ids, ggoAccountWeek(now)).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for _, row := range rows {
		if row.Bought < 0 {
			return nil, 0, errors.New(constUtil.GGOAccountWeeklyError)
		}
		counts[row.ProductID] = row.Bought
	}
	return counts, owner.AccountID, nil
}

// Lock the one shared quota row in the existing purchase transaction. A grant
// failure rolls this count back with the currency and original UUID receipt.
func useGGOAccountWeekly(tx *gorm.DB, accountID int64, product string, limit, count int64, now time.Time) error {
	if limit == 0 {
		return nil
	}
	if accountID < 1 {
		return errors.New(constUtil.GGOAccountRequired)
	}
	if count < 1 || limit < 0 || count > limit {
		return errors.New(constUtil.GGOAccountWeeklyError)
	}
	var row = db.GGOAccountWeekly{AccountID: accountID, ProductID: product, WeekStart: ggoAccountWeek(now)}
	if err := db.AccountsTable(tx, constUtil.GGOAccountWeeklyTable).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	if err := db.AccountsTable(tx, constUtil.GGOAccountWeeklyTable).Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=? AND product_id=? AND week_start=?", row.AccountID, row.ProductID, row.WeekStart).First(&row).Error; err != nil {
		return err
	}
	if row.Bought < 0 || row.Bought > limit-count || row.Bought > math.MaxInt64-count {
		return errors.New(constUtil.GGOAccountWeeklyError)
	}
	return db.AccountsTable(tx, constUtil.GGOAccountWeeklyTable).Where("account_id=? AND product_id=? AND week_start=?", row.AccountID, row.ProductID, row.WeekStart).Update("bought", row.Bought+count).Error
}

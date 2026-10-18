package service

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"time"
)

// seedP4CopyWindow retains a default-off evening template without overriding operators.
// Existing current windows and disabled instances retain their original choices and history.
func (s *GameService) seedP4CopyWindow(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	id := constUtil.P4CopyWindowPrefix + start.Format("200601")
	var exists int64
	if err := tx.Model(&db.P4CopyBenefitRule{}).Where("id=?", id).Count(&exists).Error; err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	// Existing enabled windows and explicitly closed operator windows stay authoritative.
	// Only the program's known legacy disabled placeholder permits first local adoption.
	if err := tx.Model(&db.P4CopyBenefitRule{}).Where("start_at<? AND end_at>? AND (enabled=? OR source NOT LIKE ?)", end.UnixMilli(), start.UnixMilli(), true, "本地兼容策略（默认门禁关闭）：%").Count(&exists).Error; err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	cfg := db.P4CopyBenefitRule{ID: id, Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), WeekdayMask: 127, StartMinute: 19 * 60, EndMinute: 21 * 60, CurrencyFactor: 2, DropFactor: 2, Source: constUtil.P4CopyWindowSource}
	if err := validateP4CopyBenefit(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

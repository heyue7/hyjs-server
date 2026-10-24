package service

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"time"
)

// ValidMonthSignMonth 校验自然月的固定YYYY-MM格式。
func ValidMonthSignMonth(month string) bool {
	parsed, err := time.Parse("2006-01", month)
	return err == nil && len(month) == 7 && parsed.Format("2006-01") == month
}

func monthSignMonth(month string) (string, error) {
	if month == "" {
		month = time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01")
	}
	if !ValidMonthSignMonth(month) {
		return "", constUtil.ErrAdminInput
	}
	return month, nil
}

func monthSignCalendarRules(tx *gorm.DB, month string) ([]db.MonthSignRule, bool, error) {
	var calendar db.MonthSignCalendar
	err := tx.Session(&gorm.Session{}).First(&calendar, "month_key=?", month).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var rows []db.MonthSignRule
	if json.Unmarshal(calendar.Rules, &rows) != nil || !validMonthSignRows(rows) {
		return nil, true, constUtil.ErrAdminConfiguration
	}
	return rows, true, nil
}

func validMonthSignRows(rows []db.MonthSignRule) bool {
	if len(rows) != 31 {
		return false
	}
	for i, row := range rows {
		if row.Day != int32(i+1) || row.DoubleVIP < 0 || row.DoubleVIP > 12 {
			return false
		}
	}
	return true
}

func monthSignRulesForAdmin(tx *gorm.DB, month string) ([]db.MonthSignRule, bool, error) {
	rows, configured, err := monthSignCalendarRules(tx, month)
	if err != nil || configured {
		return rows, configured, err
	}
	if err = tx.Session(&gorm.Session{}).Order("day").Find(&rows).Error; err != nil {
		return nil, false, err
	}
	if !validMonthSignRows(rows) {
		return nil, false, constUtil.ErrAdminConfiguration
	}
	return rows, false, nil
}

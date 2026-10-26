package db

import _ "embed"

// GGOAccountWeekly is shared by an account's roles and realms. One indexed row
// stores only the necessary weekly purchase count; financial receipts stay put.
type GGOAccountWeekly struct {
	AccountID int64  `gorm:"primaryKey;autoIncrement:false" json:"accountId"`
	ProductID string `gorm:"primaryKey;type:varbinary(64)" json:"productId"`
	WeekStart string `gorm:"primaryKey;type:char(10)" json:"weekStart"`
	Bought    int64  `gorm:"not null;default:0" json:"bought"`
}

func (GGOAccountWeekly) TableName() string { return "ggo_account_weekly" }

//go:embed sql/ggo_account_weekly.sql
var GGOAccountWeeklySchema string

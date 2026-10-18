package db

// MonthSignRule 记录缺失原服日历时明确采用的SQL本地签到奖历。
type MonthSignRule struct {
	Day       int32  `gorm:"primaryKey;autoIncrement:false" json:"day"`
	Reward    string `gorm:"not null" json:"reward"`
	DoubleVIP int32  `gorm:"column:double_vip;not null" json:"double_vip"`
	Enabled   bool   `gorm:"not null" json:"enabled"`
	Source    string `gorm:"not null" json:"source"`
}

func (MonthSignRule) TableName() string { return "month_sign_rules" }

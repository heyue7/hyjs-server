package db

// P10WorshipDaily 冻结每日原次数与奖励报价，跨日不借用旧日次数。
type P10WorshipDaily struct {
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day               string `gorm:"primaryKey;size:10" json:"day"`
	Limit             int64  `gorm:"column:limit_count;not null" json:"limit"`
	Reward            string `gorm:"type:text;not null" json:"reward"`
	ConfigurationHash string `gorm:"size:64;not null" json:"configuration_hash"`
	SourceHash        string `gorm:"size:64;not null" json:"source_hash"`
}

func (P10WorshipDaily) TableName() string { return "p10_worship_dailies" }

package db

// MonthSignCalendar 保存一个自然月的完整31档奖历，历史领取快照独立保留。
type MonthSignCalendar struct {
	MonthKey string `gorm:"column:month_key;primaryKey;size:7" json:"monthKey"`
	Rules    []byte `gorm:"type:blob;not null" json:"rules"`
}

func (MonthSignCalendar) TableName() string { return "month_sign_calendars" }

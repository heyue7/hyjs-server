package db

// P11LimitDailyOffer stores only published daily terms or actual accepted period point facts.
type P11LimitDailyOffer struct {
	ID         string `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	PeriodID   string `gorm:"column:period_id;type:varchar(64);not null;index" json:"periodId"`
	Day        string `gorm:"column:day;type:varchar(8);not null" json:"day"`
	StartAt    int64  `gorm:"column:start_at;not null" json:"startAt"`
	EndAt      int64  `gorm:"column:end_at;not null" json:"endAt"`
	RulesJSON  string `gorm:"column:rules_json;type:longtext;not null" json:"rulesJson"`
	PeriodHash string `gorm:"column:period_hash;type:varchar(64);not null" json:"periodHash"`
	SourceHash string `gorm:"column:source_hash;type:varchar(64);not null" json:"sourceHash"`
	CapturedAt int64  `gorm:"column:captured_at;not null" json:"capturedAt"`
}

func (P11LimitDailyOffer) TableName() string { return "p11_limit_daily_offers" }

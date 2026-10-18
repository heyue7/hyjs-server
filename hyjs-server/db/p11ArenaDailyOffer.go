package db

// P11ArenaDailyOffer stores the independently published local daily reward source.
type P11ArenaDailyOffer struct {
	Day        string `gorm:"type:varchar(8);primaryKey" json:"day"`
	StartAt    int64  `gorm:"not null" json:"startAt"`
	EndAt      int64  `gorm:"not null" json:"endAt"`
	RulesJSON  string `gorm:"column:rules_json;type:longtext;not null" json:"rulesJSON"`
	SourceHash string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CapturedAt int64  `gorm:"not null" json:"capturedAt"`
}

func (P11ArenaDailyOffer) TableName() string { return "p11_arena_daily_offers" }

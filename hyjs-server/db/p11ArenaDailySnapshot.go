package db

// P11ArenaDailySnapshot stores the independently published local daily reward source.
type P11ArenaDailySnapshot struct {
	Day        string `gorm:"type:varchar(8);primaryKey" json:"day"`
	StartAt    int64  `gorm:"not null" json:"startAt"`
	EndAt      int64  `gorm:"not null" json:"endAt"`
	OfferHash  string `gorm:"type:varchar(64);not null" json:"offerHash"`
	Status     string `gorm:"type:varchar(32);not null" json:"status"`
	Reason     string `gorm:"type:text;not null" json:"reason"`
	RanksJSON  string `gorm:"column:ranks_json;type:longtext;not null" json:"ranksJSON"`
	SourceHash string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CapturedAt int64  `gorm:"not null" json:"capturedAt"`
}

func (P11ArenaDailySnapshot) TableName() string { return "p11_arena_daily_snapshots" }

package db

// P11ArenaDailyPolicy stores the independently published local daily reward source.
type P11ArenaDailyPolicy struct {
	Permanent     bool   `gorm:"not null;default:false" json:"permanent"`
	ID            int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled       bool   `gorm:"not null" json:"enabled"`
	Version       string `gorm:"type:varchar(64);not null" json:"version"`
	Source        string `gorm:"type:text;not null" json:"source"`
	PublishedAt   int64  `gorm:"not null" json:"publishedAt"`
	StartAt       int64  `gorm:"not null" json:"startAt"`
	EndAt         int64  `gorm:"not null" json:"endAt"`
	CutoffSecond  int64  `gorm:"not null" json:"cutoffSecond"`
	MinChallenges int64  `gorm:"not null" json:"minChallenges"`
}

func (P11ArenaDailyPolicy) TableName() string { return "p11_arena_daily_policies" }

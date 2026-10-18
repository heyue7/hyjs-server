package db

// P11LimitDailyPolicy stores only published daily terms or actual accepted period point facts.
type P11LimitDailyPolicy struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement:false" json:"id"`
	Enabled       bool   `gorm:"column:enabled;not null" json:"enabled"`
	Version       string `gorm:"column:version;type:varchar(64);not null" json:"version"`
	Source        string `gorm:"column:source;type:text;not null" json:"source"`
	PublishedAt   int64  `gorm:"column:published_at;not null" json:"publishedAt"`
	StartAt       int64  `gorm:"column:start_at;not null" json:"startAt"`
	EndAt         int64  `gorm:"column:end_at;not null" json:"endAt"`
	CutoffSecond  int64  `gorm:"column:cutoff_second;not null" json:"cutoffSecond"`
	MinChallenges int64  `gorm:"column:min_challenges;not null" json:"minChallenges"`
}

func (P11LimitDailyPolicy) TableName() string { return "p11_limit_daily_policies" }

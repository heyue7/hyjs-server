package db

// P11GrabRandomPolicy is an explicitly local, prepublished finite occupation offer.
type P11GrabRandomPolicy struct {
	ID              int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	Version         string `gorm:"type:varchar(64);not null" json:"version"`
	PublishedAt     int64  `gorm:"not null" json:"publishedAt"`
	StartAt         int64  `gorm:"not null" json:"startAt"`
	EndAt           int64  `gorm:"not null" json:"endAt"`
	IntervalSeconds int64  `gorm:"not null" json:"intervalSeconds"`
	DailyLimit      int64  `gorm:"not null" json:"dailyLimit"`
	PoolJSON        string `gorm:"type:text;not null" json:"poolJSON"`
	Source          string `gorm:"type:text;not null" json:"source"`
}

func (P11GrabRandomPolicy) TableName() string { return "p11_grab_random_policies" }

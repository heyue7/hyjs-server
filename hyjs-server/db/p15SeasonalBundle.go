package db

// P15SeasonalBundle stores the native nested page's immutable child directory.
type P15SeasonalBundle struct {
	ID          string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	Kind        string `gorm:"size:16;not null" json:"kind"`
	Title       string `gorm:"size:191;not null" json:"title"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	StartAt     int64  `gorm:"not null" json:"startAt"`
	EndAt       int64  `gorm:"not null" json:"endAt"`
	RewardEndAt int64  `gorm:"not null" json:"rewardEndAt"`
	Children    []byte `gorm:"type:longblob;not null" json:"children"`
	Frozen      []byte `gorm:"type:longblob" json:"-"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P15SeasonalBundle) TableName() string { return "p15_seasonal_bundles" }

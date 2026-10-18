package db

// P11PlunderFightPolicy publishes a finite local window before its first use.
type P11PlunderFightPolicy struct {
	ID              int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	SweepEnabled    bool   `gorm:"not null;default:false" json:"sweepEnabled"`
	Version         string `gorm:"type:varchar(64);not null" json:"version"`
	PublishedAt     int64  `gorm:"not null" json:"publishedAt"`
	StartAt         int64  `gorm:"not null" json:"startAt"`
	EndAt           int64  `gorm:"not null" json:"endAt"`
	MatchCount      int32  `gorm:"not null" json:"matchCount"`
	ScanLimit       int32  `gorm:"not null" json:"scanLimit"`
	LevelDifference int32  `gorm:"not null" json:"levelDifference"`
	MatchMillis     int64  `gorm:"not null" json:"matchMillis"`
	ChanceTier      int32  `gorm:"not null" json:"chanceTier"`
	DropPPM         int64  `gorm:"not null" json:"dropPPM"`
	Source          string `gorm:"type:text;not null" json:"source"`
}

func (P11PlunderFightPolicy) TableName() string { return "p11_plunder_fight_policies" }

package db

// P14PerGameOffer preserves independent real per-game sources without changing Game1 history.
type P14PerGameOffer struct {
	SeriesID           string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	GameIndex          int32  `gorm:"primaryKey;autoIncrement:false" json:"gameIndex"`
	SeriesManifestHash string `gorm:"type:char(64);not null" json:"seriesManifestHash"`
	Policy             []byte `gorm:"type:longblob;not null" json:"policy"`
	OpenAt             int64  `gorm:"not null" json:"openAt"`
	LockAt             int64  `gorm:"not null" json:"lockAt"`
	ScheduledAt        int64  `gorm:"not null" json:"scheduledAt"`
	SourceHash         string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14PerGameOffer) TableName() string { return "p14_per_game_offers" }

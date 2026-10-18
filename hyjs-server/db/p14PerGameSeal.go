package db

// P14PerGameSeal preserves independent real per-game sources without changing Game1 history.
type P14PerGameSeal struct {
	SeriesID    string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	GameIndex   int32  `gorm:"primaryKey;autoIncrement:false" json:"gameIndex"`
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	Content     []byte `gorm:"type:longblob;not null" json:"content"`
	ContentHash string `gorm:"type:char(64);not null" json:"contentHash"`
	SealedAt    int64  `gorm:"not null" json:"sealedAt"`
}

func (P14PerGameSeal) TableName() string { return "p14_per_game_seals" }

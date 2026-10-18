package db

// P14PerGameFormation preserves independent real per-game sources without changing Game1 history.
type P14PerGameFormation struct {
	SeriesID          string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	GameIndex         int32  `gorm:"primaryKey;autoIncrement:false" json:"gameIndex"`
	ServerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Revision          int64  `gorm:"not null" json:"revision"`
	CapturedAt        int64  `gorm:"not null" json:"capturedAt"`
	FightPower        int32  `gorm:"not null" json:"fightPower"`
	Snapshot          []byte `gorm:"type:longblob;not null" json:"snapshot"`
	Battle            []byte `gorm:"type:longblob;not null" json:"battle"`
	SnapshotHash      string `gorm:"type:char(64);not null" json:"snapshotHash"`
	ConfigurationHash string `gorm:"type:char(64);not null" json:"configurationHash"`
	EngineVersion     string `gorm:"type:varchar(128);not null" json:"engineVersion"`
	ClientVersion     string `gorm:"type:varchar(128);not null" json:"clientVersion"`
	OfferHash         string `gorm:"type:char(64);not null" json:"offerHash"`
	SourceHash        string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14PerGameFormation) TableName() string { return "p14_per_game_formations" }

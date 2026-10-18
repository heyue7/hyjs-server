package db

// ServerGameClockRecovery records already consumed forward calendar recovery for a player.
type ServerGameClockRecovery struct {
	ServerID        int64 `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	PlayerID        int64 `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	HighDays        int64 `gorm:"not null" json:"highDays"`
	UpdatedAtMillis int64 `gorm:"not null" json:"updatedAt"`
}

func (ServerGameClockRecovery) TableName() string { return "server_game_clock_recoveries" }

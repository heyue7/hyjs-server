package db

// ServerGameClock is the persistent calendar for one complete game server.
type ServerGameClock struct {
	ServerID        int64 `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	OffsetDays      int64 `gorm:"not null" json:"offsetDays"`
	Revision        int64 `gorm:"not null" json:"revision"`
	HighDays        int64 `gorm:"not null" json:"highDays"`
	HighReachedAt   int64 `gorm:"not null" json:"highReachedAt"`
	UpdatedAtMillis int64 `gorm:"not null" json:"updatedAt"`
}

func (ServerGameClock) TableName() string { return "server_game_clocks" }

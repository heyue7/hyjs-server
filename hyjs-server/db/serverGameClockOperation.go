package db

// ServerGameClockOperation freezes one committed administrator request.
type ServerGameClockOperation struct {
	ServerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	RequestID string `gorm:"primaryKey;size:36" json:"requestId"`
	ActorID   int64  `gorm:"not null" json:"actorId"`
	DeltaDays int64  `gorm:"not null" json:"deltaDays"`
	Response  []byte `gorm:"type:longblob;not null" json:"-"`
	CreatedAt int64  `gorm:"not null;autoCreateTime:false" json:"createdAt"`
}

func (ServerGameClockOperation) TableName() string { return "server_game_clock_operations" }

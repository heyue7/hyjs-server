package db

// CopyResetDaily freezes price/scope and retains actual daily use per original player.
type CopyResetDaily struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Day       string `gorm:"primaryKey;type:varbinary(16)" json:"day"`
	Payload   []byte `gorm:"type:longblob;not null" json:"payload"`
	InputHash string `gorm:"type:char(64);not null" json:"inputHash"`
}

func (CopyResetDaily) TableName() string { return "copy_reset_daily" }

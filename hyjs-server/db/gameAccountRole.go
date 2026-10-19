package db

// GameAccountRole indexes the distinct role identity in each real runtime, without moving its save or receipts.
type GameAccountRole struct {
	AccountID int64  `gorm:"primaryKey;autoIncrement:false" json:"accountId"`
	ServerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	PlayerID  int64  `gorm:"not null" json:"playerId"`
	Name      string `gorm:"size:96;not null" json:"name"`
	Level     int64  `gorm:"not null" json:"level"`
}

func (GameAccountRole) TableName() string { return "game_account_roles" }

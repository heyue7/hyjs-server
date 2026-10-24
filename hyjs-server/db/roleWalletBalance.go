package db

type RoleWalletBalance struct {
	ServerID     int64 `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	PlayerID     int64 `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	AccountID    int64 `gorm:"not null;index" json:"accountId"`
	BalanceCents int64 `gorm:"not null" json:"balanceCents"`
	Revision     int64 `gorm:"not null" json:"revision"`
}

func (RoleWalletBalance) TableName() string { return "role_wallet_balances" }

// RoleWalletEntry preserves source realm/player and original legacy key for replay-safe migration.

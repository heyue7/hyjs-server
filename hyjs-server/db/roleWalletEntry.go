package db

type RoleWalletEntry struct {
	AccountID    int64   `gorm:"not null;index" json:"accountId"`
	Key          string  `gorm:"primaryKey;size:240" json:"key"`
	ServerID     int64   `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	PlayerID     int64   `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	LegacyKey    string  `gorm:"size:160;not null" json:"legacyKey"`
	Kind         string  `gorm:"size:16;not null" json:"kind"`
	DeltaCents   int64   `gorm:"not null" json:"deltaCents"`
	BalanceAfter int64   `gorm:"not null" json:"balanceAfter"`
	Revision     int64   `gorm:"not null" json:"revision"`
	OrderID      *string `gorm:"uniqueIndex:role_wallet_order;size:64" json:"orderId"`
	At           int64   `gorm:"not null;autoCreateTime:false" json:"at"`
	Source       string  `gorm:"size:64;not null" json:"source"`
	ReasonID     string  `gorm:"size:64;not null" json:"reasonId"`
	Label        string  `gorm:"size:255;not null" json:"label"`
}

func (RoleWalletEntry) TableName() string { return "role_wallet_entries" }

package db

type LocalWalletPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Version string `gorm:"size:64;not null" json:"version"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (LocalWalletPolicy) TableName() string { return "local_wallet_policies" }

type LocalWalletBalance struct {
	PlayerID     int64 `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	BalanceCents int64 `gorm:"not null" json:"balance_cents"`
	Revision     int64 `gorm:"not null" json:"revision"`
}

func (LocalWalletBalance) TableName() string { return "local_wallet_balances" }

type LocalWalletEntry struct {
	PlayerID     int64   `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Key          string  `gorm:"primaryKey;size:160" json:"key"`
	Kind         string  `gorm:"size:16;not null" json:"kind"`
	DeltaCents   int64   `gorm:"not null" json:"delta_cents"`
	BalanceAfter int64   `gorm:"not null" json:"balance_after"`
	Revision     int64   `gorm:"not null" json:"revision"`
	OrderID      *string `gorm:"uniqueIndex:local_wallet_order;size:64" json:"order_id,omitempty"`
	At           int64   `gorm:"not null;autoCreateTime:false" json:"at"`
	Source       string  `gorm:"size:64;not null" json:"source"`
	ReasonID     string  `gorm:"size:64;not null;default:''" json:"reason_id"`
	Label        string  `gorm:"size:255;not null;default:''" json:"label"`
}

func (LocalWalletEntry) TableName() string { return "local_wallet_entries" }

package db

// AccountWalletBalance is one global balance per account across all realm roles.
type AccountWalletBalance struct {
	AccountID    int64 `gorm:"primaryKey;autoIncrement:false"`
	BalanceCents int64 `gorm:"not null"`
	Revision     int64 `gorm:"not null"`
}

func (AccountWalletBalance) TableName() string { return "account_wallet_balances" }

// AccountWalletEntry preserves source realm/player and original legacy key for replay-safe migration.
type AccountWalletEntry struct {
	AccountID    int64   `gorm:"primaryKey;autoIncrement:false"`
	Key          string  `gorm:"primaryKey;size:240"`
	ServerID     int64   `gorm:"not null"`
	PlayerID     int64   `gorm:"not null"`
	LegacyKey    string  `gorm:"size:160;not null"`
	Kind         string  `gorm:"size:16;not null"`
	DeltaCents   int64   `gorm:"not null"`
	BalanceAfter int64   `gorm:"not null"`
	Revision     int64   `gorm:"not null"`
	OrderID      *string `gorm:"uniqueIndex:account_wallet_order;size:64"`
	At           int64   `gorm:"not null;autoCreateTime:false"`
	Source       string  `gorm:"size:64;not null"`
	ReasonID     string  `gorm:"size:64;not null"`
	Label        string  `gorm:"size:255;not null"`
}

func (AccountWalletEntry) TableName() string { return "account_wallet_entries" }

// AccountWalletMigration ensures a legacy per-role wallet is consolidated once, retaining immutable source rows.
type AccountWalletMigration struct {
	ServerID     int64 `gorm:"primaryKey;autoIncrement:false"`
	PlayerID     int64 `gorm:"primaryKey;autoIncrement:false"`
	AccountID    int64 `gorm:"not null"`
	BalanceCents int64 `gorm:"not null"`
	Revision     int64 `gorm:"not null"`
	EntryCount   int64 `gorm:"not null"`
}

func (AccountWalletMigration) TableName() string { return "account_wallet_migrations" }

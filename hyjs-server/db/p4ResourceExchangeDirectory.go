package db

// P4ResourceExchangeDirectory freezes original Type23 indices without creating quotas.
type P4ResourceExchangeDirectory struct {
	ID        string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	Enabled   bool   `gorm:"not null" json:"enabled"`
	StartAt   int64  `gorm:"not null" json:"startAt"`
	EndAt     int64  `gorm:"not null" json:"endAt"`
	Bindings  []byte `gorm:"type:longblob;not null" json:"bindings"`
	InputHash string `gorm:"type:char(64);not null" json:"inputHash"`
	Source    string `gorm:"type:text;not null" json:"source"`
}

func (P4ResourceExchangeDirectory) TableName() string { return "p4_resource_exchange_directories" }

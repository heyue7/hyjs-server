package db

// AdminOpeningReceipt retains per-role activity reuse and period-owned history.
type AdminOpeningReceipt struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false"`
	Timestamp    int64  `gorm:"primaryKey;autoIncrement:false"`
	OccurrenceID string `gorm:"size:64;not null;index"`
	Request      []byte `gorm:"type:longblob;not null"`
	Response     []byte `gorm:"type:longblob;not null"`
}

func (AdminOpeningReceipt) TableName() string { return "admin_opening_receipts" }

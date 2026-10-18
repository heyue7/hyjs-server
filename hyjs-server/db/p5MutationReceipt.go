package db

// P5MutationReceipt freezes a successful original protocol mutation; failed work has no receipt.
type P5MutationReceipt struct {
	PlayerID  int64  `gorm:"primaryKey"`
	Operation string `gorm:"primaryKey;size:64"`
	Timestamp int64  `gorm:"primaryKey"`
	Request   []byte `gorm:"type:longblob;not null"`
	Response  []byte `gorm:"type:longblob;not null"`
}

func (P5MutationReceipt) TableName() string { return "p5_mutation_receipts" }

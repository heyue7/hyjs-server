package db

type P9PetReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false"`
	Operation string `gorm:"primaryKey;size:64"`
	Timestamp int64  `gorm:"primaryKey;autoIncrement:false"`
	Request   []byte `gorm:"type:longblob;not null"`
	Response  []byte `gorm:"type:longblob;not null"`
}

func (P9PetReceipt) TableName() string { return "p9_pet_receipts" }

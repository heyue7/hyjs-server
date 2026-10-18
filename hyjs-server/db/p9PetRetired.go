package db

// P9PetRetired keeps the complete owned native instance before a verified sale.
type P9PetRetired struct {
	GUID      string `gorm:"primaryKey;size:64"`
	PlayerID  int64  `gorm:"not null;index"`
	PetID     string `gorm:"size:64;not null"`
	Data      []byte `gorm:"type:longblob;not null"`
	Operation string `gorm:"size:64;not null"`
	Timestamp int64  `gorm:"not null"`
	Coin      int64  `gorm:"not null"`
	Coinpet   int64  `gorm:"not null;default:0"`
	Source    string `gorm:"type:longtext;not null"`
}

func (P9PetRetired) TableName() string { return "p9_pet_retired" }

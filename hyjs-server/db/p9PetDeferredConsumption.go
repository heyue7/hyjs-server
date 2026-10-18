package db

// P9PetDeferredConsumption consumes a previously recorded copy award once while preserving its original receipt.
type P9PetDeferredConsumption struct {
	PlayerID     int64  `gorm:"primaryKey;not null"`
	SourceKey    string `gorm:"primaryKey;size:191;not null"`
	GUID         string `gorm:"primaryKey;size:64;not null"`
	Applied      int64  `gorm:"not null"`
	Capped       int64  `gorm:"not null"`
	PolicySource string `gorm:"type:longtext;not null"`
}

func (P9PetDeferredConsumption) TableName() string { return "p9_pet_deferred_consumptions" }

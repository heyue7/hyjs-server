package db

// P9PetTalentAllocation retains actual random choices and their immutable published policy.
type P9PetTalentAllocation struct {
	GUID       string `gorm:"primaryKey;size:64"`
	PlayerID   int64  `gorm:"not null;index"`
	PolicyHash string `gorm:"size:64;not null"`
	Draws      []byte `gorm:"type:longblob;not null"`
	Source     string `gorm:"type:longtext;not null"`
}

func (P9PetTalentAllocation) TableName() string { return "p9_pet_talent_allocations" }

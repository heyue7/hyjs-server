package db

// P9PetInvestment records exact paid resources and consumed donor snapshots.
type P9PetInvestment struct {
	GUID     string `gorm:"primaryKey;size:64"`
	PlayerID int64  `gorm:"not null;index"`
	Costs    []byte `gorm:"type:longblob;not null"`
	Donors   []byte `gorm:"type:longblob;not null"`
	Source   string `gorm:"type:longtext;not null"`
}

func (P9PetInvestment) TableName() string { return "p9_pet_investments" }

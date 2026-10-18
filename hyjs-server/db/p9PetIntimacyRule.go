package db

// P9PetIntimacyRule publishes the named local distribution separately from original tables.
type P9PetIntimacyRule struct {
	Quality int64  `gorm:"primaryKey"`
	Enabled bool   `gorm:"not null"`
	Version int64  `gorm:"not null"`
	Pool    []byte `gorm:"type:longblob;not null"`
	Source  string `gorm:"type:longtext;not null"`
}

func (P9PetIntimacyRule) TableName() string { return "p9_pet_intimacy_rules" }

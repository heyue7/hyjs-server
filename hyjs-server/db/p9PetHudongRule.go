package db

// P9PetHudongRule keeps declared local preference policy separately from original constants.
type P9PetHudongRule struct {
	ID      string `gorm:"primaryKey;size:32" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Input   []byte `gorm:"type:longblob;not null" json:"input"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P9PetHudongRule) TableName() string { return "p9_pet_hudong_rules" }

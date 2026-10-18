package db

// P9PetHudongState owns real per-GUID attempts and frozen preference during one window.
type P9PetHudongState struct {
	GUID      string `gorm:"primaryKey;size:64" json:"guid"`
	PlayerID  int64  `gorm:"not null;index" json:"playerId"`
	WindowAt  int64  `gorm:"not null" json:"windowAt"`
	LastAt    int64  `gorm:"not null" json:"lastAt"`
	Input     []byte `gorm:"type:longblob;not null" json:"input"`
	InputHash string `gorm:"size:64;not null" json:"inputHash"`
	Favorite  int32  `gorm:"not null" json:"favorite"`
	Counts    []byte `gorm:"type:longblob;not null" json:"counts"`
	Earned    int64  `gorm:"not null" json:"earned"`
	Applied   int64  `gorm:"not null" json:"applied"`
	Capped    int64  `gorm:"not null" json:"capped"`
}

func (P9PetHudongState) TableName() string { return "p9_pet_hudong_states" }

package db

type P9PetState struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false"`
	FirstGranted  bool   `gorm:"not null"`
	NextFree      int64  `gorm:"not null"`
	FreeRemaining int32  `gorm:"not null"`
	MustTimes     int32  `gorm:"not null"`
	GuardGUID     string `gorm:"size:64;not null"`
	Source        string `gorm:"type:longtext;not null"`
}

func (P9PetState) TableName() string { return "p9_pet_states" }

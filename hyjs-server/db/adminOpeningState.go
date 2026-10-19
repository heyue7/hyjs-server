package db

// AdminOpeningState retains per-role activity reuse and period-owned history.
type AdminOpeningState struct {
	OccurrenceID string `gorm:"primaryKey;size:64"`
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false"`
	TaskID       int32  `gorm:"primaryKey;autoIncrement:false"`
	Achieved     bool   `gorm:"not null"`
	Progress     int32  `gorm:"not null"`
	Claims       int32  `gorm:"not null"`
}

func (AdminOpeningState) TableName() string { return "admin_opening_states" }

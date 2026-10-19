package db

// AdminOpeningCounter retains per-role activity reuse and period-owned history.
type AdminOpeningCounter struct {
	OccurrenceID string `gorm:"primaryKey;size:64"`
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false"`
	CounterKey   string `gorm:"primaryKey;size:96"`
	Value        int32  `gorm:"not null"`
}

func (AdminOpeningCounter) TableName() string { return "admin_opening_counters" }

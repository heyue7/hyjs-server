package db

// AdminOpeningStock retains per-role activity reuse and period-owned history.
type AdminOpeningStock struct {
	OccurrenceID string `gorm:"primaryKey;size:64"`
	TaskID       int32  `gorm:"primaryKey;autoIncrement:false"`
	UsedStock    int32  `gorm:"not null"`
}

func (AdminOpeningStock) TableName() string { return "admin_opening_stocks" }

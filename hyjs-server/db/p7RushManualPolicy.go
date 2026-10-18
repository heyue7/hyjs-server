package db

// P7RushManualPolicy enables only newly created paid Trial runs. Existing
// automatic calculations cannot acquire this driver permission retroactively.
type P7RushManualPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null;default:false" json:"enabled"`
	Source  string `gorm:"type:varchar(255);not null" json:"source"`
}

func (P7RushManualPolicy) TableName() string { return "p7_rush_manual_policies" }

package db

// P7RushScopedPolicy opens only the separately certified automatic-record family.
// The default remains off; enabling this row cannot bypass the compiled P6 proof.
type P7RushScopedPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Source  string `gorm:"type:varchar(255);not null" json:"source"`
}

func (P7RushScopedPolicy) TableName() string { return "p7_rush_scoped_policies" }

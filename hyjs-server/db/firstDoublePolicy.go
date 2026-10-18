package db

// FirstDoublePolicy preserves the operator's explicit lifetime SKU policy.
type FirstDoublePolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Version string `gorm:"size:64;not null" json:"version"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (FirstDoublePolicy) TableName() string { return "first_double_policies" }

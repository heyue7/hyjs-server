package db

// CopySpecialRule owns the explicit local cadence, without interpreting private triples.
type CopySpecialRule struct {
	ID      string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Cycle   int64  `gorm:"not null" json:"cycle"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (CopySpecialRule) TableName() string { return "copy_special_rules" }

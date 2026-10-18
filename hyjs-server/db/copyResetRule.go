package db

// CopyResetRule stores only the unknown original daily scope as a named local choice.
type CopyResetRule struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Scope   string `gorm:"type:varbinary(32);not null" json:"scope"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (CopyResetRule) TableName() string { return "copy_reset_rules" }

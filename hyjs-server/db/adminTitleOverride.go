package db

// AdminTitleOverride only changes future original-title qualification and grants.
type AdminTitleOverride struct {
	ID         string `gorm:"primaryKey;type:varchar(64)"`
	Param      string `gorm:"type:varchar(64);not null"`
	Effects    string `gorm:"type:text;not null"`
	EffectTime int64  `gorm:"not null"`
	Show       bool   `gorm:"not null"`
	Enabled    bool   `gorm:"not null"`
}

func (AdminTitleOverride) TableName() string { return "admin_title_overrides" }

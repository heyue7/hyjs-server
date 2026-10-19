package db

// AdminActivityTemplateOwner retains per-role activity reuse and period-owned history.
type AdminActivityTemplateOwner struct {
	ServerID     int64  `gorm:"primaryKey;autoIncrement:false"`
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false"`
	TemplateID   string `gorm:"primaryKey;size:64"`
	OccurrenceID string `gorm:"size:64;not null"`
	CreatedAt    int64  `gorm:"not null;autoCreateTime:false"`
}

func (AdminActivityTemplateOwner) TableName() string { return "admin_activity_template_owners" }

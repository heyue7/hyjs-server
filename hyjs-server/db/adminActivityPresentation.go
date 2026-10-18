package db

// AdminActivityPresentation 关联原业务实例与受限美术模板，归档不删除历史业务记录。
type AdminActivityPresentation struct {
	ActivityID      string `gorm:"primaryKey;size:160" json:"activityId"`
	Mode            string `gorm:"size:64;not null" json:"mode"`
	TemplateID      string `gorm:"size:96;not null" json:"templateId"`
	Name            string `gorm:"size:160;not null" json:"name"`
	Archived        bool   `gorm:"not null" json:"archived"`
	UpdatedAtMillis int64  `gorm:"not null" json:"updatedAt"`
}

func (AdminActivityPresentation) TableName() string { return "admin_activity_presentations" }

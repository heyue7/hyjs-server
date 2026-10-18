package db

// AdminFieldTemplate 保存全局可复用的完整野外资源组合。
type AdminFieldTemplate struct {
	ID            string `gorm:"primaryKey;size:64" json:"id"`
	Name          string `gorm:"size:120;not null" json:"name"`
	Scene         string `gorm:"size:64;index;not null" json:"scene"`
	Configuration string `gorm:"type:longtext;not null" json:"-"`
	Revision      int64  `gorm:"not null" json:"revision"`
	UpdatedAt     int64  `gorm:"not null" json:"updatedAt"`
}

func (AdminFieldTemplate) TableName() string { return "admin_field_templates" }

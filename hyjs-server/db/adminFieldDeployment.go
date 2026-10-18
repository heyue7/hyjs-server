package db

// AdminFieldDeployment 按区服和地图冻结已发布资源组合，修改全局方案不会暗改线上。
type AdminFieldDeployment struct {
	ServerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	Scene         string `gorm:"primaryKey;size:64" json:"scene"`
	TemplateID    string `gorm:"size:64;not null" json:"templateId"`
	Enabled       bool   `gorm:"not null" json:"enabled"`
	Configuration string `gorm:"type:longtext;not null" json:"-"`
	Revision      int64  `gorm:"not null" json:"revision"`
	UpdatedAt     int64  `gorm:"not null" json:"updatedAt"`
}

func (AdminFieldDeployment) TableName() string { return "admin_field_deployments" }

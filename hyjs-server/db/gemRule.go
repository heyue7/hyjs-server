package db

// GemRule 只定义原 APK 缺失的提炼六色晶石策略和七彩自动孔位选择；费用及属性读取原表。
type GemRule struct {
	ID              int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	CrystalPerBatch int32  `gorm:"not null" json:"crystalPerBatch"`
	Source          string `gorm:"not null" json:"source"`
}

func (GemRule) TableName() string { return "gem_rules" }

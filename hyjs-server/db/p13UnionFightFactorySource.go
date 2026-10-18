package db

// P13UnionFightFactorySource 发布原表缺失的初始战车HP；不把具名本地数值伪称原服务器公式。
type P13UnionFightFactorySource struct {
	Day         string  `gorm:"primaryKey;type:varchar(10)" json:"day"`
	FactoryID   int64   `gorm:"primaryKey;autoIncrement:false" json:"factory_id"`
	Enabled     bool    `gorm:"not null;default:false" json:"enabled"`
	ChariotHP   float64 `gorm:"not null" json:"chariot_hp"`
	PublishedAt int64   `gorm:"not null" json:"published_at"`
	Source      string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightFactorySource) TableName() string { return "p13_union_fight_factory_sources" }

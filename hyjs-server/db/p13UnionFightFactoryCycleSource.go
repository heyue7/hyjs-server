package db

// P13UnionFightFactoryCycleSource 提前发布连续造车的具名本地产能上限，原120秒保持；默认无来源。
type P13UnionFightFactoryCycleSource struct {
	Day            string `gorm:"primaryKey;type:char(10)" json:"day"`
	FactoryID      int64  `gorm:"primaryKey;autoIncrement:false" json:"factory_id"`
	Enabled        bool   `gorm:"not null;default:false" json:"enabled"`
	MaxGenerations int64  `gorm:"not null" json:"max_generations"`
	MaxAlive       int64  `gorm:"not null" json:"max_alive"`
	PublishedAt    int64  `gorm:"not null" json:"published_at"`
	Source         string `gorm:"not null;type:text" json:"source"`
	Rule           string `gorm:"type:varchar(64);not null;default:''" json:"rule,omitempty"`
	BufferCapacity int64  `gorm:"not null;default:0" json:"buffer_capacity,omitempty"`
}

func (P13UnionFightFactoryCycleSource) TableName() string {
	return "p13_union_fight_factory_cycle_sources"
}

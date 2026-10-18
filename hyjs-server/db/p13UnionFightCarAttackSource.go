package db

// P13UnionFightCarAttackSource 保存原战车实际攻门的提前具名本地配置及持久计时。
type P13UnionFightCarAttackSource struct {
	Day         string  `gorm:"primaryKey;type:char(10)" json:"day"`
	FactoryID   int64   `gorm:"primaryKey;autoIncrement:false" json:"factory_id"`
	Enabled     bool    `gorm:"not null;default:false" json:"enabled"`
	HPPercent   int64   `gorm:"not null" json:"hp_percent"`
	Radius      float64 `gorm:"not null" json:"radius"`
	PublishedAt int64   `gorm:"not null" json:"published_at"`
	Source      string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightCarAttackSource) TableName() string { return "p13_union_fight_car_attack_sources" }

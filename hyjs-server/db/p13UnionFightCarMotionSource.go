package db

// P13UnionFightCarMotionSource 提前发布原战车目录缺失的具名本地行进速度。
type P13UnionFightCarMotionSource struct {
	Day         string  `gorm:"primaryKey;type:char(10)" json:"day"`
	FactoryID   int64   `gorm:"primaryKey;autoIncrement:false" json:"factory_id"`
	Enabled     bool    `gorm:"not null;default:false" json:"enabled"`
	Speed       float64 `gorm:"not null" json:"speed"`
	PublishedAt int64   `gorm:"not null" json:"published_at"`
	Source      string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightCarMotionSource) TableName() string { return "p13_union_fight_car_motion_sources" }

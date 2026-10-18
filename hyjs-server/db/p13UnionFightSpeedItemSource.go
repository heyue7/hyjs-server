package db

// P13UnionFightSpeedItemSource 保存原015友方3米/5秒冲锋buff及提前具名倍率和未知资格的本地策略。
type P13UnionFightSpeedItemSource struct {
	Day           string  `gorm:"primaryKey;type:char(10)" json:"day"`
	ItemID        string  `gorm:"primaryKey;type:varchar(64)" json:"item_id"`
	Enabled       bool    `gorm:"not null;default:false" json:"enabled"`
	Multiplier    float64 `gorm:"not null" json:"multiplier"`
	AuthorityMask int64   `gorm:"not null" json:"authority_mask"`
	PublishedAt   int64   `gorm:"not null" json:"published_at"`
	Source        string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightSpeedItemSource) TableName() string { return "p13_union_fight_speed_item_sources" }

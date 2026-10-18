package db

// P13UnionFightWarDrumSource 保存原014/totem1与提前明确发布的本地模型、范围、持续及攻击属性加成。
type P13UnionFightWarDrumSource struct {
	Day            string  `gorm:"primaryKey;type:char(10)" json:"day"`
	ItemID         string  `gorm:"primaryKey;type:varchar(64)" json:"item_id"`
	Enabled        bool    `gorm:"not null;default:false" json:"enabled"`
	AttackPercent  int64   `gorm:"not null" json:"attack_percent"`
	Radius         float64 `gorm:"not null" json:"radius"`
	DurationMillis int64   `gorm:"not null" json:"duration_millis"`
	AuthorityMask  int64   `gorm:"not null" json:"authority_mask"`
	ModelName      string  `gorm:"type:varchar(128);not null" json:"model_name"`
	ModelScale     float64 `gorm:"not null" json:"model_scale"`
	PublishedAt    int64   `gorm:"not null" json:"published_at"`
	Source         string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightWarDrumSource) TableName() string { return "p13_union_fight_war_drum_sources" }

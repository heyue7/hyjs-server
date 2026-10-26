package db

// P13UnionFightHealAuraSource 保存原013回复法阵的原buff009五秒及提前发布具名本地团队治疗策略。
type P13UnionFightHealAuraSource struct {
	EffectRule    string  `gorm:"type:varchar(64);not null;default:''" json:"effect_rule,omitempty"`
	Day           string  `gorm:"primaryKey;type:char(10)" json:"day"`
	ItemID        string  `gorm:"primaryKey;type:varchar(64)" json:"item_id"`
	Enabled       bool    `gorm:"not null;default:false" json:"enabled"`
	HPPercent     int64   `gorm:"not null" json:"hp_percent"`
	Radius        float64 `gorm:"not null" json:"radius"`
	TickMillis    int64   `gorm:"not null" json:"tick_millis"`
	AuthorityMask int64   `gorm:"not null" json:"authority_mask"`
	PublishedAt   int64   `gorm:"not null" json:"published_at"`
	Source        string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightHealAuraSource) TableName() string { return "p13_union_fight_heal_aura_sources" }

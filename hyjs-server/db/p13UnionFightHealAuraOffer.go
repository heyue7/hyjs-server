package db

// P13UnionFightHealAuraOffer 保存原013回复法阵的原buff009五秒及提前发布具名本地团队治疗策略。
type P13UnionFightHealAuraOffer struct {
	MatchID        string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	ItemID         string  `gorm:"primaryKey;type:varchar(64)" json:"item_id"`
	HPPercent      int64   `gorm:"not null" json:"hp_percent"`
	Radius         float64 `gorm:"not null" json:"radius"`
	TickMillis     int64   `gorm:"not null" json:"tick_millis"`
	AuthorityMask  int64   `gorm:"not null" json:"authority_mask"`
	DurationMillis int64   `gorm:"not null" json:"duration_millis"`
	CooldownMillis int64   `gorm:"not null" json:"cooldown_millis"`
	Source         string  `gorm:"type:text;not null" json:"source"`
	SourceHash     string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightHealAuraOffer) TableName() string { return "p13_union_fight_heal_aura_offers" }

package db

// P13UnionFightWarDrumOffer 保存原014/totem1与提前明确发布的本地模型、范围、持续及攻击属性加成。
type P13UnionFightWarDrumOffer struct {
	MatchID        string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	ItemID         string  `gorm:"primaryKey;type:varchar(64)" json:"item_id"`
	AttackPercent  int64   `gorm:"not null" json:"attack_percent"`
	Radius         float64 `gorm:"not null" json:"radius"`
	DurationMillis int64   `gorm:"not null" json:"duration_millis"`
	AuthorityMask  int64   `gorm:"not null" json:"authority_mask"`
	CooldownMillis int64   `gorm:"not null" json:"cooldown_millis"`
	ModelName      string  `gorm:"type:varchar(128);not null" json:"model_name"`
	ModelScale     float64 `gorm:"not null" json:"model_scale"`
	Source         string  `gorm:"type:text;not null" json:"source"`
	SourceHash     string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightWarDrumOffer) TableName() string { return "p13_union_fight_war_drum_offers" }

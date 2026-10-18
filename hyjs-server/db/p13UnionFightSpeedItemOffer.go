package db

// P13UnionFightSpeedItemOffer 保存原015友方3米/5秒冲锋buff及提前具名倍率和未知资格的本地策略。
type P13UnionFightSpeedItemOffer struct {
	MatchID        string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	ItemID         string  `gorm:"primaryKey;type:varchar(64)" json:"item_id"`
	Multiplier     float64 `gorm:"not null" json:"multiplier"`
	AuthorityMask  int64   `gorm:"not null" json:"authority_mask"`
	Radius         float64 `gorm:"not null" json:"radius"`
	DurationMillis int64   `gorm:"not null" json:"duration_millis"`
	CooldownMillis int64   `gorm:"not null" json:"cooldown_millis"`
	Source         string  `gorm:"type:text;not null" json:"source"`
	SourceHash     string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightSpeedItemOffer) TableName() string { return "p13_union_fight_speed_item_offers" }

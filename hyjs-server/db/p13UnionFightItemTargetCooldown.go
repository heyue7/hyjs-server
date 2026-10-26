package db

// P13UnionFightItemTargetCooldown isolates one actual entity and effect family.
type P13UnionFightItemTargetCooldown struct {
	MatchID      string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	TargetGUID   int64  `gorm:"primaryKey;autoIncrement:false" json:"targetGUID"`
	EffectFamily string `gorm:"primaryKey;type:varchar(32)" json:"effectFamily"`
	Until        int64  `gorm:"not null" json:"until"`
	Rule         string `gorm:"type:varchar(64);not null" json:"rule"`
	OfferHash    string `gorm:"type:char(64);not null" json:"offerHash"`
	SourceHash   string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightItemTargetCooldown) TableName() string {
	return "p13_union_fight_item_target_cooldowns"
}

package db

// P13UnionFightCarItemOffer 按场冻结原敌方5米目标和具名实际伤害比例。
type P13UnionFightCarItemOffer struct {
	BudgetRule           string  `gorm:"type:varchar(64);not null;default:''" json:"budgetRule,omitempty"`
	TeamUseLimit         int64   `gorm:"not null;default:0" json:"teamUseLimit,omitempty"`
	TargetCooldownMillis int64   `gorm:"not null;default:0" json:"targetCooldownMillis,omitempty"`
	EffectFamily         string  `gorm:"type:varchar(32);not null;default:''" json:"effectFamily,omitempty"`
	MatchID              string  `gorm:"primaryKey;type:char(64)" json:"matchId"`
	ItemID               string  `gorm:"primaryKey;type:varchar(64)" json:"itemId"`
	HPPercent            int64   `gorm:"not null" json:"hpPercent"`
	Radius               float64 `gorm:"not null" json:"radius"`
	CooldownMillis       int64   `gorm:"not null" json:"cooldownMillis"`
	Source               string  `gorm:"type:text;not null" json:"source"`
	SourceHash           string  `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightCarItemOffer) TableName() string { return "p13_union_fight_car_item_offers" }

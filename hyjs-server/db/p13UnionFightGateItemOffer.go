package db

// P13UnionFightGateItemOffer 冻结原5米本方/敌方最近门规则与具名HP效果量。
type P13UnionFightGateItemOffer struct {
	BudgetRule           string  `gorm:"type:varchar(64);not null;default:''" json:"budgetRule,omitempty"`
	TeamUseLimit         int64   `gorm:"not null;default:0" json:"teamUseLimit,omitempty"`
	TargetCooldownMillis int64   `gorm:"not null;default:0" json:"targetCooldownMillis,omitempty"`
	EffectFamily         string  `gorm:"type:varchar(32);not null;default:''" json:"effectFamily,omitempty"`
	MatchID              string  `gorm:"primaryKey;type:char(64)" json:"matchId"`
	ItemID               string  `gorm:"primaryKey;type:varchar(64)" json:"itemId"`
	HPPercent            int64   `gorm:"not null" json:"hpPercent"`
	Enemy                bool    `gorm:"not null" json:"enemy"`
	Radius               float64 `gorm:"not null" json:"radius"`
	CooldownMillis       int64   `gorm:"not null" json:"cooldownMillis"`
	Source               string  `gorm:"type:text;not null" json:"source"`
	SourceHash           string  `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightGateItemOffer) TableName() string { return "p13_union_fight_gate_item_offers" }

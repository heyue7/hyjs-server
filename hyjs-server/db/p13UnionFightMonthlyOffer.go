package db

// P13UnionFightMonthlyOffer is armed before the month starts and never created late.
type P13UnionFightMonthlyOffer struct {
	Month     string `gorm:"primaryKey;type:char(7)" json:"month"`
	StartAt   int64  `gorm:"not null" json:"start_at"`
	CutoffAt  int64  `gorm:"not null" json:"cutoff_at"`
	Rules     []byte `gorm:"type:longblob;not null" json:"-"`
	RulesHash string `gorm:"type:char(64);not null" json:"rules_hash"`
	FrozenAt  int64  `gorm:"not null" json:"frozen_at"`
}

func (P13UnionFightMonthlyOffer) TableName() string { return "p13_union_fight_monthly_offers" }

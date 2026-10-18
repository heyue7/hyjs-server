package db

// P13UnionFightWeeklyOffer freezes the first pre-start publication for a real weekly group.
type P13UnionFightWeeklyOffer struct {
	WeekStart    string `gorm:"primaryKey;type:char(10)" json:"week_start"`
	Group        string `gorm:"primaryKey;type:char(1)" json:"group"`
	Rules        []byte `gorm:"type:longblob;not null" json:"-"`
	RulesHash    string `gorm:"type:char(64);not null" json:"rules_hash"`
	FrozenAt     int64  `gorm:"not null" json:"frozen_at"`
	FirstMatchID string `gorm:"type:char(64);not null" json:"first_match_id"`
}

func (P13UnionFightWeeklyOffer) TableName() string { return "p13_union_fight_weekly_offers" }

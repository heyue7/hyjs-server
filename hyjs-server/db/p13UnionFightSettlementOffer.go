package db

// P13UnionFightSettlementOffer 保存具名运营零战斗终局及其真实发奖来源。
type P13UnionFightSettlementOffer struct {
	MatchID    string `gorm:"primaryKey;type:char(64)" json:"match_id"`
	Rules      []byte `gorm:"type:longblob;not null" json:"rules"`
	RulesHash  string `gorm:"type:char(64);not null" json:"rules_hash"`
	RosterHash string `gorm:"type:char(64);not null" json:"roster_hash"`
	FrozenAt   int64  `gorm:"not null" json:"frozen_at"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightSettlementOffer) TableName() string { return "p13_union_fight_settlement_offers" }

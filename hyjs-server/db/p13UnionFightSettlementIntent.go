package db

// P13UnionFightSettlementIntent 保存具名运营零战斗终局及其真实发奖来源。
type P13UnionFightSettlementIntent struct {
	MatchID    string `gorm:"primaryKey;type:char(64)" json:"match_id"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	TargetID   int64  `gorm:"primaryKey;autoIncrement:false" json:"target_id"`
	Stamp      int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	ObservedAt int64  `gorm:"not null" json:"observed_at"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightSettlementIntent) TableName() string { return "p13_union_fight_settlement_intents" }

package db

// P13UnionFightBattleAuditOffer 冻结原14/40回合/原0.8实际抵达和具名审计输入；未知原种子/倍率明确local。
type P13UnionFightBattleAuditOffer struct {
	MatchID              string   `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	SeedSalt             int64    `gorm:"not null" json:"seed_salt"`
	ArriveRange          float64  `gorm:"not null" json:"arrive_range"`
	MaxRounds            int32    `gorm:"not null" json:"max_rounds"`
	AttackerDamageFactor *float64 `gorm:"" json:"attacker_damage_factor"`
	DefenderDamageFactor *float64 `gorm:"" json:"defender_damage_factor"`
	Source               string   `gorm:"not null;type:text" json:"source"`
	SourceHash           string   `gorm:"not null;type:char(64)" json:"source_hash"`
}

func (P13UnionFightBattleAuditOffer) TableName() string { return "p13_union_fight_battle_audit_offers" }

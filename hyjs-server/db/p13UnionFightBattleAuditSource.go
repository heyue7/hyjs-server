package db

// P13UnionFightBattleAuditSource 提前发布具名本地种子盐/伤害倍率，原14仅实际计算审计，默认没有来源。
type P13UnionFightBattleAuditSource struct {
	Day                  string   `gorm:"primaryKey;type:char(10)" json:"day"`
	Enabled              bool     `gorm:"not null;default:false" json:"enabled"`
	SeedSalt             int64    `gorm:"not null" json:"seed_salt"`
	AttackerDamageFactor *float64 `gorm:"" json:"attacker_damage_factor"`
	DefenderDamageFactor *float64 `gorm:"" json:"defender_damage_factor"`
	PublishedAt          int64    `gorm:"not null" json:"published_at"`
	Source               string   `gorm:"not null;type:text" json:"source"`
}

func (P13UnionFightBattleAuditSource) TableName() string {
	return "p13_union_fight_battle_audit_sources"
}

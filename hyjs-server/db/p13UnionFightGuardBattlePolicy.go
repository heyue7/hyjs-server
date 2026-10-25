package db

// P13UnionFightGuardBattlePolicy preserves the independent guard lifecycle; existing audit records remain untouched.
type P13UnionFightGuardBattlePolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(64);not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
	PublishedAt int64  `gorm:"not null;default:0" json:"published_at"`
	StartsAt    int64  `gorm:"not null;default:0" json:"starts_at"`
	EndsAt      int64  `gorm:"not null;default:0" json:"ends_at"`
}

func (P13UnionFightGuardBattlePolicy) TableName() string {
	return "p13_union_fight_guard_battle_policies"
}

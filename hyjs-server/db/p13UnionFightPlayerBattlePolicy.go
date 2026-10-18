package db

// P13UnionFightPlayerBattlePolicy publishes an independent ordinary-player policy before a new match; disabled by default.
type P13UnionFightPlayerBattlePolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(64);not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"published_at"`
	StartsAt    int64  `gorm:"not null" json:"starts_at"`
	EndsAt      int64  `gorm:"not null" json:"ends_at"`
	ReviveRule  string `gorm:"type:varchar(64);not null" json:"revive_rule"`
}

func (P13UnionFightPlayerBattlePolicy) TableName() string {
	return "p13_union_fight_player_battle_policies"
}

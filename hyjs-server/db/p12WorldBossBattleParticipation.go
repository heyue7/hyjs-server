package db

// P12WorldBossBattleParticipation 按角色保存原免费次数及恢复锚点。
type P12WorldBossBattleParticipation struct {
	PlayerID    int64 `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Remaining   int64 `gorm:"not null" json:"remaining"`
	RecoveredAt int64 `gorm:"not null" json:"recovered_at"`
}

func (P12WorldBossBattleParticipation) TableName() string {
	return "p12_world_boss_battle_participations"
}

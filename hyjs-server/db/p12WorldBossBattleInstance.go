package db

// P12WorldBossBattleInstance 为一个原窗口保存共享HP和唯一最后一击事实。
type P12WorldBossBattleInstance struct {
	ID         string `gorm:"primaryKey;type:varchar(64)" json:"id"`
	TotalHP    int64  `gorm:"not null" json:"total_hp"`
	CurrentHP  int64  `gorm:"not null" json:"current_hp"`
	KillerID   int64  `gorm:"not null" json:"killer_id"`
	KilledAt   int64  `gorm:"not null" json:"killed_at"`
	PolicyName string `gorm:"type:varchar(96);not null" json:"policy_name"`
}

func (P12WorldBossBattleInstance) TableName() string { return "p12_world_boss_battle_instances" }

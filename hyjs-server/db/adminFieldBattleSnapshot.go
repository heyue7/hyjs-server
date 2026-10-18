package db

// AdminFieldBattleSnapshot 冻结开战时资源方案的掉落与刷新，保护进行中战斗。
type AdminFieldBattleSnapshot struct {
	FightID    string `gorm:"primaryKey;size:64" json:"fightId"`
	PlayerID   int64  `gorm:"index;not null" json:"playerId"`
	Scene      string `gorm:"size:64;not null" json:"scene"`
	Reward     string `gorm:"type:longtext;not null" json:"reward"`
	CooldownMS int64  `gorm:"not null" json:"cooldownMs"`
}

func (AdminFieldBattleSnapshot) TableName() string { return "admin_field_battle_snapshots" }

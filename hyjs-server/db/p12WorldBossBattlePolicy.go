package db

// P12WorldBossBattlePolicy 保存具名本地结算规则，安装后默认关闭。
type P12WorldBossBattlePolicy struct {
	ID             int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled        bool   `gorm:"not null" json:"enabled"`
	PolicyName     string `gorm:"type:varchar(96);not null" json:"policy_name"`
	Source         string `gorm:"type:text;not null" json:"source"`
	EconomyEnabled bool   `gorm:"not null;default:false" json:"economy_enabled"`
	RewardsEnabled bool   `gorm:"not null;default:false" json:"rewards_enabled"`
	EconomySource  string `gorm:"type:text" json:"economy_source"`
}

func (P12WorldBossBattlePolicy) TableName() string { return "p12_world_boss_battle_policies" }

package db

// P12WorldBossAward 冻结本地奖档、加权抽取后的实际资源与领奖中心投递回执。
type P12WorldBossAward struct {
	InstanceID string `gorm:"primaryKey;type:varchar(64)" json:"instance_id"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Kind       int64  `gorm:"primaryKey;autoIncrement:false" json:"kind"`
	PolicyName string `gorm:"type:varchar(96);not null" json:"policy_name"`
	Reward     string `gorm:"type:text;not null" json:"reward"`
	DeliveryID int64  `gorm:"not null" json:"delivery_id"`
	ExpiresAt  int64  `gorm:"not null" json:"expires_at"`
}

func (P12WorldBossAward) TableName() string { return "p12_world_boss_awards" }

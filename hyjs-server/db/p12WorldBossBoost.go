package db

// P12WorldBossBoost 每周期每角色每种加成仅可付费购买一次，冻结原属性定义。
type P12WorldBossBoost struct {
	InstanceID string `gorm:"primaryKey;type:varchar(64)" json:"instance_id"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	PropertyID string `gorm:"primaryKey;type:varchar(64)" json:"property_id"`
	Property   string `gorm:"type:varchar(128);not null" json:"property"`
	Kind       int64  `gorm:"not null" json:"kind"`
	Paid       int64  `gorm:"not null" json:"paid"`
	CreatedAt  int64  `gorm:"not null" json:"created_at"`
}

func (P12WorldBossBoost) TableName() string { return "p12_world_boss_boosts" }

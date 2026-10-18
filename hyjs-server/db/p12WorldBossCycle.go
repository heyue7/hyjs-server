package db

// P12WorldBossCycle 一个原日历窗口对应一个真实UUID，保存完整定义和出生选择，未发生权威伤害不形成击杀/领奖事实。
type P12WorldBossCycle struct {
	ID             string `gorm:"primaryKey;type:varchar(64)" json:"id"`
	WindowKey      string `gorm:"type:varchar(64);uniqueIndex;not null" json:"window_key"`
	State          string `gorm:"type:varchar(16);not null" json:"state"`
	StartsAt       int64  `gorm:"not null" json:"starts_at"`
	EndsAt         int64  `gorm:"not null" json:"ends_at"`
	DefinitionJSON string `gorm:"type:longtext;not null" json:"definition_json"`
	DefinitionHash string `gorm:"type:varchar(64);not null" json:"definition_hash"`
	Level          int64  `gorm:"not null" json:"level"`
	PositionID     int64  `gorm:"not null" json:"position_id"`
	CreatedAt      int64  `gorm:"not null" json:"created_at"`
	ActivatedAt    int64  `gorm:"not null" json:"activated_at"`
	EndedAt        int64  `gorm:"not null" json:"ended_at"`
}

func (P12WorldBossCycle) TableName() string { return "p12_world_boss_cycles" }

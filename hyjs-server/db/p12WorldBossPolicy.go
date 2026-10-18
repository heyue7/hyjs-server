package db

// P12WorldBossPolicy 原世界首领全定义冻结，公共查询默认关闭，初始等级/指定出生点具名本地选择。
type P12WorldBossPolicy struct {
	ID             int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled        bool   `gorm:"not null" json:"enabled"`
	Level          int64  `gorm:"not null" json:"level"`
	PositionID     int64  `gorm:"not null" json:"position_id"`
	DefinitionJSON string `gorm:"type:longtext;not null" json:"definition_json"`
	DefinitionHash string `gorm:"type:varchar(64);not null" json:"definition_hash"`
	Source         string `gorm:"type:text;not null" json:"source"`
}

func (P12WorldBossPolicy) TableName() string { return "p12_world_boss_policies" }

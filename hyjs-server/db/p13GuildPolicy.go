package db

// P13GuildPolicy 保存独立公会管理和经济运营门禁；默认不开放写入。
type P13GuildPolicy struct {
	ID                int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	ManagementEnabled bool   `gorm:"not null" json:"managementEnabled"`
	EconomyEnabled    bool   `gorm:"not null" json:"economyEnabled"`
	ChatEnabled       bool   `gorm:"not null" json:"chatEnabled"`
	Source            string `gorm:"type:text;not null" json:"source"`
}

func (P13GuildPolicy) TableName() string { return "p13_guild_policies" }

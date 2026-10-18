package db

// P13GuildApply 保存真实入会申请，同一玩家对同一公会仅一行。
type P13GuildApply struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	GuildID   string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	CreatedAt int64  `gorm:"not null" json:"createdAt"`
}

func (P13GuildApply) TableName() string { return "p13_guild_applies" }

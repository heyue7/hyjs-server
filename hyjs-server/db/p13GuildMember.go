package db

// P13GuildMember 以玩家为主键防止多会；离会保留冷却与每日消费事实。
type P13GuildMember struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	GuildID      string `gorm:"type:varchar(64);not null;index" json:"guildId"`
	Role         int32  `gorm:"not null" json:"role"`
	JoinedAt     int64  `gorm:"not null" json:"joinedAt"`
	QuitUntil    int64  `gorm:"not null" json:"quitUntil"`
	DonateDay    string `gorm:"type:varchar(10);not null" json:"donateDay"`
	TodayCoin    int64  `gorm:"not null" json:"todayCoin"`
	TodayBadge   int32  `gorm:"not null" json:"todayBadge"`
	Contribution int32  `gorm:"not null" json:"contribution"`
	LastLoginDay string `gorm:"type:varchar(10);not null" json:"lastLoginDay"`
	LastLoginAt  int64  `gorm:"not null" json:"lastLoginAt"`
	StaminaDay   string `gorm:"type:varchar(10);not null" json:"staminaDay"`
}

func (P13GuildMember) TableName() string { return "p13_guild_members" }

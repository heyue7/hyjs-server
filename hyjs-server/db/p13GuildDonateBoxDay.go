package db

// P13GuildDonateBoxDay freezes an enabled rule when the day's first real donation commits.
type P13GuildDonateBoxDay struct {
	GuildID     string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	Day         string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	RulesJSON   string `gorm:"type:text;not null" json:"rulesJson"`
	RulesHash   string `gorm:"type:char(64);not null" json:"rulesHash"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
	CreatedAt   int64  `gorm:"not null" json:"createdAt"`
	Score       int32  `gorm:"not null" json:"score"`
}

func (P13GuildDonateBoxDay) TableName() string { return "p13_guild_donate_box_days" }

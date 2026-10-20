package db

// P13GuildDonateBoxPolicy explicitly names the local daily allocation rule.
type P13GuildDonateBoxPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	RulesJSON   string `gorm:"type:text" json:"rulesJson"`
	PublishedAt int64  `gorm:"not null;default:0" json:"publishedAt"`
}

func (P13GuildDonateBoxPolicy) TableName() string { return "p13_guild_donate_box_policies" }

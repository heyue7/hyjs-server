package db

// P13GuildGiftPolicy belongs only to authenticated ordinary guild monthly gifts.
type P13GuildGiftPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	StartsAt    int64  `gorm:"not null" json:"starts_at"`
	PublishedAt int64  `gorm:"not null" json:"published_at"`
	Rule        string `gorm:"type:varchar(96);not null" json:"rule"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13GuildGiftPolicy) TableName() string { return "p13_guild_gift_policies" }

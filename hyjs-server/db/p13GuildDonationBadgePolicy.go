package db

// P13GuildDonationBadgePolicy names the opt-in paid donation badge source.
type P13GuildDonationBadgePolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	CoinCost    int32  `gorm:"not null" json:"coinCost"`
	BadgeAmount int32  `gorm:"not null" json:"badgeAmount"`
	DailyLimit  int64  `gorm:"not null" json:"dailyLimit"`
	PublishedAt int64  `gorm:"not null;default:0" json:"publishedAt"`
}

func (P13GuildDonationBadgePolicy) TableName() string { return "p13_guild_donation_badge_policies" }

package db

// P13GuildDonationBadgeSource binds a real debit to one immutable P1 delivery.
type P13GuildDonationBadgeSource struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false;uniqueIndex:idx_p13_donation_badge_day,priority:1" json:"playerId"`
	Stamp       int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	Day         string `gorm:"type:varchar(10);not null;uniqueIndex:idx_p13_donation_badge_day,priority:2" json:"day"`
	Ordinal     int64  `gorm:"not null;uniqueIndex:idx_p13_donation_badge_day,priority:3" json:"ordinal"`
	GuildID     string `gorm:"type:varchar(64);not null" json:"guildId"`
	JoinedAt    int64  `gorm:"not null" json:"joinedAt"`
	CoinCost    int32  `gorm:"not null" json:"coinCost"`
	BadgeAmount int32  `gorm:"not null" json:"badgeAmount"`
	DailyLimit  int64  `gorm:"not null" json:"dailyLimit"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
	CreatedAt   int64  `gorm:"not null" json:"createdAt"`
	DeliveryID  int64  `gorm:"not null;uniqueIndex" json:"deliveryId"`
	DeliveryKey string `gorm:"type:varchar(191);not null;uniqueIndex" json:"deliveryKey"`
	SourceHash  string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13GuildDonationBadgeSource) TableName() string { return "p13_guild_donation_badge_sources" }

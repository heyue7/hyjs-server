package db

// P13GuildDonateBoxSource records only a donation debited in the same transaction.
type P13GuildDonateBoxSource struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Stamp      int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	GuildID    string `gorm:"type:varchar(64);not null;index:idx_p13_box_source_day,priority:1" json:"guildId"`
	Day        string `gorm:"type:varchar(10);not null;index:idx_p13_box_source_day,priority:2" json:"day"`
	JoinedAt   int64  `gorm:"not null" json:"joinedAt"`
	DonateType string `gorm:"type:varchar(16);not null" json:"donateType"`
	Amount     int32  `gorm:"not null" json:"amount"`
	Score      int32  `gorm:"not null" json:"score"`
	CreatedAt  int64  `gorm:"not null" json:"createdAt"`
	SourceHash string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13GuildDonateBoxSource) TableName() string { return "p13_guild_donate_box_sources" }

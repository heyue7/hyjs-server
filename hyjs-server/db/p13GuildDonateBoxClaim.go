package db

// P13GuildDonateBoxClaim prevents one daily tier being claimed again after moving guilds.
type P13GuildDonateBoxClaim struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Day          string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	Ordinal      int64  `gorm:"primaryKey;autoIncrement:false" json:"ordinal"`
	GuildID      string `gorm:"type:varchar(64);not null" json:"guildId"`
	JoinedAt     int64  `gorm:"not null" json:"joinedAt"`
	BoxID        string `gorm:"type:char(64);not null" json:"boxId"`
	Stamp        int64  `gorm:"not null" json:"stamp"`
	Score        int32  `gorm:"not null" json:"score"`
	Threshold    int32  `gorm:"not null" json:"threshold"`
	Reward       string `gorm:"type:varchar(128);not null" json:"reward"`
	Version      string `gorm:"type:varchar(128);not null" json:"version"`
	CreatedAt    int64  `gorm:"not null" json:"createdAt"`
	Response     []byte `gorm:"type:longblob;not null" json:"-"`
	ResponseHash string `gorm:"type:char(64);not null" json:"responseHash"`
}

func (P13GuildDonateBoxClaim) TableName() string { return "p13_guild_donate_box_claims" }

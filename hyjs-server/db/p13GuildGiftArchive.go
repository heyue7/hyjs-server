package db

// P13GuildGiftArchive belongs only to authenticated ordinary guild monthly gifts.
type P13GuildGiftArchive struct {
	GuildID     string `gorm:"primaryKey;type:varchar(64)" json:"guild_id"`
	Score       int32  `gorm:"not null" json:"score"`
	GuildRaw    []byte `gorm:"type:longblob;not null" json:"guild_raw"`
	GuildHash   string `gorm:"type:char(64);not null" json:"guild_hash"`
	CollectedAt int64  `gorm:"not null" json:"collected_at"`
	Source      string `gorm:"type:text;not null" json:"source"`
	SourceHash  string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildGiftArchive) TableName() string { return "p13_guild_gift_archives" }

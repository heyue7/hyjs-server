package db

// P13GuildGiftRule belongs only to authenticated ordinary guild monthly gifts.
type P13GuildGiftRule struct {
	Month           string `gorm:"primaryKey;type:char(7)" json:"month"`
	StartAt         int64  `gorm:"not null" json:"start_at"`
	EndAt           int64  `gorm:"not null" json:"end_at"`
	Credit          int32  `gorm:"not null" json:"credit"`
	Levels          []byte `gorm:"type:longblob;not null" json:"levels"`
	LevelsHash      string `gorm:"type:char(64);not null" json:"levels_hash"`
	Config          []byte `gorm:"type:longblob;not null" json:"config"`
	ConfigHash      string `gorm:"type:char(64);not null" json:"config_hash"`
	LoginConfig     []byte `gorm:"type:longblob;not null" json:"login_config"`
	LoginConfigHash string `gorm:"type:char(64);not null" json:"login_config_hash"`
	FrozenAt        int64  `gorm:"not null" json:"frozen_at"`
	Source          string `gorm:"type:text;not null" json:"source"`
	SourceHash      string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildGiftRule) TableName() string { return "p13_guild_gift_rules" }

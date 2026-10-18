package db

// P13GuildPrayerRoll is one actual contribution debit and immutable random
// effect. Later pool changes do not reroll existing prayer effects.
type P13GuildPrayerRoll struct {
	PlayerID         int64  `gorm:"primaryKey;autoIncrement:false;uniqueIndex:idx_p13_prayer_day_order,priority:1" json:"player_id"`
	Stamp            int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	GuildID          string `gorm:"type:varchar(64);not null" json:"guild_id"`
	Day              string `gorm:"type:char(10);not null;uniqueIndex:idx_p13_prayer_day_order,priority:2" json:"day"`
	Ordinal          int64  `gorm:"not null;uniqueIndex:idx_p13_prayer_day_order,priority:3" json:"ordinal"`
	AppliedAt        int64  `gorm:"not null" json:"applied_at"`
	ExpiresAt        int64  `gorm:"not null" json:"expires_at"`
	ContributionCost int32  `gorm:"not null" json:"contribution_cost"`
	DailyLimit       int32  `gorm:"not null" json:"daily_limit"`
	DurationHours    int32  `gorm:"not null" json:"duration_hours"`
	PoolVersion      string `gorm:"type:varchar(128);not null" json:"pool_version"`
	PoolJSON         string `gorm:"type:text;not null" json:"pool_json"`
	PoolHash         string `gorm:"type:char(64);not null" json:"pool_hash"`
	PublishedAt      int64  `gorm:"not null" json:"published_at"`
	Draw             int64  `gorm:"not null" json:"draw"`
	ChosenIndex      int64  `gorm:"not null" json:"chosen_index"`
	Effect           []byte `gorm:"type:blob;not null" json:"effect"`
	EffectHash       string `gorm:"type:char(64);not null" json:"effect_hash"`
	Source           string `gorm:"type:varchar(1024);not null" json:"source"`
	SourceHash       string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildPrayerRoll) TableName() string { return "p13_guild_prayer_rolls" }

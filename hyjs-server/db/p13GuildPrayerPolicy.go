package db

// P13GuildPrayerPolicy is an explicitly published local pool. The original
// private-server probabilities were not present in the recovered APK.
type P13GuildPrayerPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null;default:''" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PoolJSON    string `gorm:"type:text;not null" json:"pool_json"`
	PoolHash    string `gorm:"type:char(64);not null;default:''" json:"pool_hash"`
	PublishedAt int64  `gorm:"not null;default:0" json:"published_at"`
}

func (P13GuildPrayerPolicy) TableName() string { return "p13_guild_prayer_policies" }

package db

// P13GuildTechPolicy publishes an explicit local building directory. It does not override original costs/effects.
type P13GuildTechPolicy struct {
	ID            int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled       bool   `gorm:"not null;default:false" json:"enabled"`
	Version       string `gorm:"type:varchar(128);not null;default:''" json:"version"`
	DirectoryJSON string `gorm:"type:text;not null" json:"directory_json"`
	DirectoryHash string `gorm:"type:char(64);not null" json:"directory_hash"`
	PublishedAt   int64  `gorm:"not null;default:0" json:"published_at"`
	Source        string `gorm:"type:varchar(1024);not null" json:"source"`
}

func (P13GuildTechPolicy) TableName() string { return "p13_guild_tech_policies" }

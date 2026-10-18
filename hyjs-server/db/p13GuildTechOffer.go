package db

// P13GuildTechOffer freezes one guild's actual first-upgrade directory and all recovered original rows.
type P13GuildTechOffer struct {
	GuildID          string `gorm:"primaryKey;type:varchar(64)" json:"guild_id"`
	Version          string `gorm:"type:varchar(128);not null" json:"version"`
	DirectoryJSON    string `gorm:"type:text;not null" json:"directory_json"`
	DirectoryHash    string `gorm:"type:char(64);not null" json:"directory_hash"`
	OriginalRowsJSON string `gorm:"type:text;not null" json:"original_rows_json"`
	OriginalRowsHash string `gorm:"type:char(64);not null" json:"original_rows_hash"`
	OriginalClient   string `gorm:"type:varchar(128);not null" json:"original_client"`
	PublishedAt      int64  `gorm:"not null" json:"published_at"`
	CapturedAt       int64  `gorm:"not null" json:"captured_at"`
	Source           string `gorm:"type:varchar(1024);not null" json:"source"`
	SourceHash       string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildTechOffer) TableName() string { return "p13_guild_tech_offers" }

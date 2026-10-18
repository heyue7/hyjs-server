package db

// P13GuildTechUpgrade is an immutable real level transition and original guild-fund debit.
type P13GuildTechUpgrade struct {
	GuildID         string `gorm:"primaryKey;type:varchar(64)" json:"guild_id"`
	Type            string `gorm:"primaryKey;column:tech_type;type:varchar(16)" json:"tech_type"`
	Level           int64  `gorm:"primaryKey;autoIncrement:false" json:"level"`
	Building        string `gorm:"type:varchar(32);not null" json:"building"`
	PlayerID        int64  `gorm:"not null" json:"player_id"`
	Stamp           int64  `gorm:"not null" json:"stamp"`
	AppliedAt       int64  `gorm:"not null" json:"applied_at"`
	Coin            int64  `gorm:"not null" json:"coin"`
	Badge           int32  `gorm:"not null" json:"badge"`
	Prop            string `gorm:"type:varchar(128);not null" json:"prop"`
	OfferHash       string `gorm:"type:char(64);not null" json:"offer_hash"`
	OriginalRowHash string `gorm:"type:char(64);not null" json:"original_row_hash"`
	SourceHash      string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildTechUpgrade) TableName() string { return "p13_guild_tech_upgrades" }

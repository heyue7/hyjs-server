package db

// P12FieldPVPState separates permanent notoriety, daily accepted kills and loot,
// a persisted presentation lease, and prison/redemption facts. Profile locks own it.
type P12FieldPVPState struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Kills       int32  `gorm:"not null" json:"kills"`
	Day         string `gorm:"size:10;not null" json:"day"`
	DailyKills  int32  `gorm:"not null" json:"dailyKills"`
	LootCoin    int64  `gorm:"not null" json:"lootCoin"`
	BusyUntil   int64  `gorm:"not null" json:"busyUntil"`
	LastFightID string `gorm:"size:64;not null" json:"lastFightId"`
	JailUntil   int64  `gorm:"not null" json:"jailUntil"`
	RedeemCount int64  `gorm:"not null" json:"redeemCount"`
	Source      string `gorm:"type:longtext;not null" json:"source"`
}

func (P12FieldPVPState) TableName() string { return "p12_field_pvp_states" }

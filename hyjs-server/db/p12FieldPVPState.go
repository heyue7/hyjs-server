package db

// P12FieldPVPWallClock identifies timestamps written from the server wall clock.
// Version zero predates clock provenance and cannot be converted safely.
const P12FieldPVPWallClock int32 = 1

// P12FieldPVPState separates permanent notoriety, daily accepted kills and loot,
// a persisted presentation lease, and prison/redemption facts. Profile locks own it.
type P12FieldPVPState struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Kills         int32  `gorm:"not null" json:"kills"`
	Day           string `gorm:"size:10;not null" json:"day"`
	DailyKills    int32  `gorm:"not null" json:"dailyKills"`
	LootCoin      int64  `gorm:"not null" json:"lootCoin"`
	BusyUntil     int64  `gorm:"not null;index:idx_p12_field_pvp_busy" json:"busyUntil"`
	ClockVersion  int32  `gorm:"not null;default:0" json:"clockVersion"`
	RealBusyUntil int64  `gorm:"not null;default:0;index:idx_p12_field_pvp_real_busy" json:"realBusyUntil"`
	LastFightID   string `gorm:"size:64;not null" json:"lastFightId"`
	JailUntil     int64  `gorm:"not null" json:"jailUntil"`
	RealJailUntil int64  `gorm:"not null;default:0" json:"realJailUntil"`
	RedeemCount   int64  `gorm:"not null" json:"redeemCount"`
	Source        string `gorm:"type:longtext;not null" json:"source"`
}

func (P12FieldPVPState) TableName() string { return "p12_field_pvp_states" }

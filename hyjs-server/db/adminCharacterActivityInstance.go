package db

// AdminCharacterActivityInstance freezes one serving-realm character's schedule
// into its own native consumer. Profile identity, never global account age,
// determines this ownership and the original immutable creation-day anchor.
type AdminCharacterActivityInstance struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	OccurrenceID  string `gorm:"primaryKey;size:64" json:"occurrenceId"`
	ServerID      int64  `gorm:"not null;index:idx_character_activity_scope" json:"serverId"`
	ConsumerID    string `gorm:"size:160;not null;uniqueIndex" json:"consumerId"`
	ConsumerKey   string `gorm:"size:128;not null;uniqueIndex" json:"-"`
	Mode          string `gorm:"size:64;not null;index:idx_character_activity_mode" json:"mode"`
	StartAt       int64  `gorm:"not null" json:"startAt"`
	EndAt         int64  `gorm:"not null" json:"endAt"`
	ClaimEndAt    int64  `gorm:"not null" json:"claimEndAt"`
	SourceVersion string `gorm:"size:64;not null" json:"-"`
	FormJSON      []byte `gorm:"type:longblob;not null" json:"-"`
}

func (AdminCharacterActivityInstance) TableName() string { return "admin_character_activity_instances" }

package db

// P11LimitFightMatch stores only current real mode20 publication, frozen input or accepted facts.
type P11LimitFightMatch struct {
	ID                string `gorm:"primaryKey;type:varchar(64)" json:"id"`
	PeriodID          string `gorm:"type:varchar(64);not null;index" json:"period_id"`
	PlayerID          int64  `gorm:"not null;index:p11_limit_match_lookup,priority:1" json:"player_id"`
	OpponentID        int64  `gorm:"not null;index:p11_limit_match_lookup,priority:2" json:"opponent_id"`
	OwnScore          int64  `gorm:"not null" json:"own__score"`
	PeerScore         int64  `gorm:"not null" json:"peer__score"`
	OfferVersion      string `gorm:"type:varchar(64);not null" json:"offer__version"`
	OfferHash         string `gorm:"type:varchar(64);not null" json:"offer__hash"`
	PeriodHash        string `gorm:"type:varchar(64);not null" json:"period__hash"`
	Attacker          []byte `gorm:"type:longblob;not null" json:"attacker"`
	Defender          []byte `gorm:"type:longblob;not null" json:"defender"`
	Seed              int64  `gorm:"not null" json:"seed"`
	ConfigurationHash string `gorm:"type:varchar(64);not null" json:"configuration__hash"`
	CapabilityHash    string `gorm:"type:varchar(64);not null" json:"capability__hash"`
	CreatedAt         int64  `gorm:"not null" json:"created__at"`
	ExpiresAt         int64  `gorm:"not null;index:p11_limit_match_lookup,priority:3" json:"expires__at"`
	SourceHash        string `gorm:"type:varchar(64);not null" json:"source__hash"`
}

func (P11LimitFightMatch) TableName() string { return "p11_limit_fight_matches" }

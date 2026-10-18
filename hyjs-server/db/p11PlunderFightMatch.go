package db

// P11PlunderFightMatch binds an actual holder and both complete original teams.
type P11PlunderFightMatch struct {
	ID                string `gorm:"primaryKey;type:varchar(64)" json:"id"`
	PlayerID          int64  `gorm:"not null;index:idx_p11_plunder_owner,priority:1" json:"playerId"`
	OpponentID        int64  `gorm:"not null;index:idx_p11_plunder_owner,priority:2" json:"opponentId"`
	FragmentID        string `gorm:"type:varchar(64);not null;index:idx_p11_plunder_owner,priority:3" json:"fragmentId"`
	OfferVersion      string `gorm:"type:varchar(64);not null" json:"offerVersion"`
	OfferHash         string `gorm:"type:varchar(64);not null" json:"offerHash"`
	Info              []byte `gorm:"type:blob;not null" json:"info"`
	Attacker          []byte `gorm:"type:longblob;not null" json:"attacker"`
	Defender          []byte `gorm:"type:longblob;not null" json:"defender"`
	Seed              int64  `gorm:"not null" json:"seed"`
	ConfigurationHash string `gorm:"type:varchar(64);not null" json:"configurationHash"`
	CapabilityHash    string `gorm:"type:varchar(64);not null" json:"capabilityHash"`
	SourceHash        string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CreatedAt         int64  `gorm:"not null" json:"createdAt"`
	ExpiresAt         int64  `gorm:"not null;index:idx_p11_plunder_owner,priority:4" json:"expiresAt"`
}

func (P11PlunderFightMatch) TableName() string { return "p11_plunder_fight_matches" }

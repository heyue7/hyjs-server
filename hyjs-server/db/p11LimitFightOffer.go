package db

// P11LimitFightOffer stores only current real mode20 publication, frozen input or accepted facts.
type P11LimitFightOffer struct {
	Version    string `gorm:"primaryKey;type:varchar(64)" json:"version"`
	Snapshot   string `gorm:"type:longtext;not null" json:"snapshot"`
	SourceHash string `gorm:"type:varchar(64);not null" json:"source__hash"`
	CapturedAt int64  `gorm:"not null" json:"captured__at"`
}

func (P11LimitFightOffer) TableName() string { return "p11_limit_fight_offers" }

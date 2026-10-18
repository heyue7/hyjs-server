package db

// P11GrabFightOffer freezes one published policy version and its original cost/session rules.
type P11GrabFightOffer struct {
	Version    string `gorm:"primaryKey;type:varchar(64)" json:"version"`
	Snapshot   string `gorm:"type:longtext;not null" json:"snapshot"`
	SourceHash string `gorm:"type:varchar(64);not null" json:"source_hash"`
	CapturedAt int64  `gorm:"not null" json:"captured_at"`
}

func (P11GrabFightOffer) TableName() string { return "p11_grab_fight_offers" }

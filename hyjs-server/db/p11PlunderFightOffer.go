package db

// P11PlunderFightOffer freezes the complete prepublished local policy once.
type P11PlunderFightOffer struct {
	Version    string `gorm:"primaryKey;type:varchar(64)" json:"version"`
	Snapshot   string `gorm:"type:longtext;not null" json:"snapshot"`
	SourceHash string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CapturedAt int64  `gorm:"not null" json:"capturedAt"`
}

func (P11PlunderFightOffer) TableName() string { return "p11_plunder_fight_offers" }

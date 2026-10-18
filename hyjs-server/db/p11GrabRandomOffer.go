package db

// P11GrabRandomOffer fixes a monthly publication before earning new real time.
type P11GrabRandomOffer struct {
	Month      string `gorm:"type:varchar(6);primaryKey" json:"month"`
	Snapshot   string `gorm:"type:longtext;not null" json:"snapshot"`
	SourceHash string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CapturedAt int64  `gorm:"not null" json:"capturedAt"`
}

func (P11GrabRandomOffer) TableName() string { return "p11_grab_random_offers" }

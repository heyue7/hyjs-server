package db

// P11ArenaPrizeOffer freezes the first real eligible publication for a Beijing
// month. Later policy edits cannot reset its limits or change earned prizes.
type P11ArenaPrizeOffer struct {
	Month      string `gorm:"type:varchar(6);primaryKey" json:"month"`
	Snapshot   string `gorm:"type:longtext;not null" json:"snapshot"`
	SourceHash string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CapturedAt int64  `gorm:"not null" json:"capturedAt"`
}

func (P11ArenaPrizeOffer) TableName() string { return "p11_arena_prize_offers" }

package db

// P14FinalDraw is an immutable list of actual entrants and deterministic local first-round pairs.
type P14FinalDraw struct {
	SeasonIndex int32  `gorm:"primaryKey" json:"seasonIndex"`
	Content     []byte `gorm:"type:longblob;not null" json:"-"`
	ContentHash string `gorm:"type:char(64);not null" json:"contentHash"`
	ObservedAt  int64  `gorm:"not null" json:"observedAt"`
}

func (P14FinalDraw) TableName() string { return "p14_final_draws" }

package db

// P13UnionFightMonthlyCutoff freezes membership and chairman identity before any
// post-cutoff guild mutation while holding the same global guild gate.
type P13UnionFightMonthlyCutoff struct {
	Month        string `gorm:"primaryKey;type:char(7)" json:"month"`
	OfferHash    string `gorm:"type:char(64);not null" json:"offer_hash"`
	CutoffAt     int64  `gorm:"not null" json:"cutoff_at"`
	CapturedAt   int64  `gorm:"not null" json:"captured_at"`
	Members      []byte `gorm:"type:longblob;not null" json:"-"`
	Guilds       []byte `gorm:"type:longblob;not null" json:"-"`
	SnapshotHash string `gorm:"type:char(64);not null" json:"snapshot_hash"`
	Source       string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightMonthlyCutoff) TableName() string { return "p13_union_fight_monthly_cutoffs" }

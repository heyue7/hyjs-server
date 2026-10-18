package db

// P13UnionFightMonthlyResult seals verified terminal/factory prestige and cutoff
// recipients before P1 delivery; incomplete actual rounds produce no result.
type P13UnionFightMonthlyResult struct {
	Month       string `gorm:"primaryKey;type:char(7)" json:"month"`
	OfferHash   string `gorm:"type:char(64);not null" json:"offer_hash"`
	CutoffHash  string `gorm:"type:char(64);not null" json:"cutoff_hash"`
	Rankings    []byte `gorm:"type:longblob;not null" json:"-"`
	Awards      []byte `gorm:"type:longblob;not null" json:"-"`
	ResultHash  string `gorm:"type:char(64);not null" json:"result_hash"`
	SettledAt   int64  `gorm:"not null" json:"settled_at"`
	DeliveredAt int64  `gorm:"not null;default:0" json:"delivered_at"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightMonthlyResult) TableName() string { return "p13_union_fight_monthly_results" }

package db

// P13UnionFightWeeklyResult seals actual rankings and participant entitlements before P1 delivery.
type P13UnionFightWeeklyResult struct {
	ByeDirectory []byte `gorm:"type:longblob" json:"bye_directory,omitempty"`
	WeekStart    string `gorm:"primaryKey;type:char(10)" json:"week_start"`
	Group        string `gorm:"primaryKey;type:char(1)" json:"group"`
	OfferHash    string `gorm:"type:char(64);not null" json:"offer_hash"`
	Rankings     []byte `gorm:"type:longblob;not null" json:"-"`
	Awards       []byte `gorm:"type:longblob;not null" json:"-"`
	ResultHash   string `gorm:"type:char(64);not null" json:"result_hash"`
	SettledAt    int64  `gorm:"not null" json:"settled_at"`
	DeliveredAt  int64  `gorm:"not null;default:0" json:"delivered_at"`
	Source       string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightWeeklyResult) TableName() string { return "p13_union_fight_weekly_results" }

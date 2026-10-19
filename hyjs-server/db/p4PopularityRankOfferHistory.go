package db

// P4PopularityRankOfferHistory preserves the exact catalogue body accepted by
// historical rank claims while current operational prices remain editable.
type P4PopularityRankOfferHistory struct {
	OfferHash string `gorm:"primaryKey;type:char(64)" json:"offerHash"`
	RuleID    string `gorm:"type:varbinary(64);not null;index" json:"ruleId"`
	Payload   []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P4PopularityRankOfferHistory) TableName() string { return "p4_popularity_rank_offer_history" }

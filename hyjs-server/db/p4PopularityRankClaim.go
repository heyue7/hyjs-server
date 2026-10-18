package db

// P4PopularityRankClaim retains the actual accepted ranking witness and frozen
// offer consumed by this one-time local exchange, independently of daily quota.
type P4PopularityRankClaim struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RuleID    string `gorm:"primaryKey;type:varbinary(64)" json:"ruleId"`
	ProductID string `gorm:"primaryKey;type:varbinary(64)" json:"productId"`
	Rank      int64  `gorm:"not null" json:"rank"`
	RankProof string `gorm:"type:char(64);not null" json:"rankProof"`
	OfferHash string `gorm:"type:char(64);not null" json:"offerHash"`
	Source    string `gorm:"type:text;not null" json:"source"`
	Stamp     int64  `gorm:"not null" json:"stamp"`
	ClaimedAt int64  `gorm:"not null" json:"claimedAt"`
}

func (P4PopularityRankClaim) TableName() string { return "p4_popularity_rank_claims" }

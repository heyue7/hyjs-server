package db

// P14PromotionRankVersion is both an immutable score version and its import receipt.
type P14PromotionRankVersion struct {
	SeasonIndex  int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	Revision     int64  `gorm:"primaryKey;autoIncrement:false" json:"revision"`
	Content      []byte `gorm:"type:longblob;not null" json:"-"`
	ContentHash  string `gorm:"type:char(64);not null" json:"content_hash"`
	PreviousHash string `gorm:"type:char(64);not null" json:"previous_hash"`
	CapturedAt   int64  `gorm:"not null" json:"captured_at"`
	ImportedAt   int64  `gorm:"not null" json:"imported_at"`
}

func (P14PromotionRankVersion) TableName() string { return "p14_promotion_rank_versions" }

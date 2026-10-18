package db

// P14PromotionRankDelivery keeps exact signed version bytes until the peer commits them.
type P14PromotionRankDelivery struct {
	ID                  string `gorm:"primaryKey;type:char(64)" json:"id"`
	SeasonIndex         int32  `gorm:"not null;index:p14_rank_pending,priority:1" json:"season_index"`
	SourceServerID      int64  `gorm:"not null" json:"source_server_id"`
	DestinationServerID int64  `gorm:"not null;index:p14_rank_pending,priority:2" json:"destination_server_id"`
	Revision            int64  `gorm:"not null;index:p14_rank_pending,priority:3" json:"revision"`
	Payload             []byte `gorm:"type:longblob;not null" json:"-"`
	PayloadHash         string `gorm:"type:char(64);not null" json:"payload_hash"`
	Confirmed           bool   `gorm:"not null;default:false;index:p14_rank_pending,priority:4" json:"confirmed"`
	AttemptCount        int64  `gorm:"not null;default:0" json:"attempt_count"`
	CreatedAt           int64  `gorm:"not null;autoCreateTime:false" json:"created_at"`
	ConfirmedAt         int64  `gorm:"not null;default:0" json:"confirmed_at"`
}

func (P14PromotionRankDelivery) TableName() string { return "p14_promotion_rank_deliveries" }

package db

// PaymentLuxuryOffer 保存P3支付域的独立持久化状态。
type PaymentLuxuryOffer struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	OfferID  int64  `gorm:"primaryKey;autoIncrement:false" json:"offer_id"`
	StartAt  int64  `gorm:"not null" json:"start_at"`
	EndAt    int64  `gorm:"not null" json:"end_at"`
	Cents    int64  `gorm:"not null" json:"cents"`
	Reward   string `gorm:"type:text;not null" json:"reward"`
	Claimed  bool   `gorm:"not null" json:"claimed"`
}

func (PaymentLuxuryOffer) TableName() string { return "p3_payment_luxury_offers" }

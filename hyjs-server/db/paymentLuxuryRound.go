package db

// PaymentLuxuryRound preserves one expired, unqualified offer window.
type PaymentLuxuryRound struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false"`
	OfferID        int64  `gorm:"primaryKey;autoIncrement:false"`
	StartAt        int64  `gorm:"primaryKey;autoIncrement:false"`
	EndAt          int64  `gorm:"not null"`
	Cents          int64  `gorm:"not null"`
	Reward         string `gorm:"type:text;not null"`
	ConfirmedCents int64  `gorm:"not null"`
}

func (PaymentLuxuryRound) TableName() string { return "p3_payment_luxury_rounds" }

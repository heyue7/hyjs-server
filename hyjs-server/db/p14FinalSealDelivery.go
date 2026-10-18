package db

// P14FinalSealDelivery retries the exact signed seal independently of purpose3 delivery order.
type P14FinalSealDelivery struct {
	ID                  string `gorm:"type:char(64);primaryKey" json:"id"`
	DestinationServerID int64  `gorm:"not null;index" json:"destinationServerId"`
	Payload             []byte `gorm:"type:longblob;not null" json:"-"`
	PayloadHash         string `gorm:"type:char(64);not null" json:"payloadHash"`
	CreatedAt           int64  `gorm:"not null" json:"createdAt"`
	Confirmed           bool   `gorm:"not null;default:false;index" json:"confirmed"`
	ConfirmedAt         int64  `gorm:"not null" json:"confirmedAt"`
	AttemptCount        int64  `gorm:"not null;default:0" json:"attemptCount"`
}

func (P14FinalSealDelivery) TableName() string { return "p14_final_seal_deliveries" }

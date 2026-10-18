package db

// PaymentConfirmation 保存可信测试adapter的确认源键与金额，跨订单重复源键拒绝。
type PaymentConfirmation struct {
	EventID     string `gorm:"primaryKey;size:128" json:"event_id"`
	OrderID     string `gorm:"size:64;not null" json:"order_id"`
	PayloadHash string `gorm:"size:64;not null" json:"payload_hash"`
	Outcome     string `gorm:"size:16;not null" json:"outcome"`
	At          int64  `gorm:"not null" json:"at"`
	Source      string `gorm:"size:32;not null" json:"source"`
}

func (PaymentConfirmation) TableName() string { return "p3_payment_confirmations" }

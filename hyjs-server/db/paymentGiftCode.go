package db

// PaymentGiftCode 保存P3支付域的独立持久化状态。
type PaymentGiftCode struct {
	Hash      string `gorm:"primaryKey;size:64" json:"-"`
	Reward    string `gorm:"type:text;not null" json:"reward"`
	ExpiresAt int64  `gorm:"not null" json:"expires_at"`
	Limit     int64  `gorm:"not null" json:"limit"`
	Used      int64  `gorm:"not null" json:"used"`
	DemoOnly  bool   `gorm:"not null" json:"demo_only"`
	Source    string `gorm:"type:text;not null" json:"source"`
}

func (PaymentGiftCode) TableName() string { return "p3_payment_gift_codes" }

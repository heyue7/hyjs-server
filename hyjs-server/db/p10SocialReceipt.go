package db

// P10SocialReceipt 原社交协议独立持久化域。
type P10SocialReceipt struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false"`
	Domain      string `gorm:"primaryKey;type:varbinary(32)"`
	Stamp       int64  `gorm:"primaryKey;autoIncrement:false"`
	RequestHash string `gorm:"type:char(64);not null"`
	Response    []byte `gorm:"type:longblob;not null"`
}

func (P10SocialReceipt) TableName() string { return "p10_social_receipts" }

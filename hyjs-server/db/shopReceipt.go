package db

// ShopReceipt 绑定最后一次完整购买请求和回包，网络重试沿用已提交结果。
type ShopReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Timestamp int64  `gorm:"not null" json:"timestamp"`
	Request   []byte `gorm:"type:longblob" json:"request"`
	Response  []byte `gorm:"type:longblob" json:"response"`
}

func (ShopReceipt) TableName() string { return "shop_receipts" }

package db

// GGOShopReceipt 按角色永久保留成功购买的UUID及原始响应。
type GGOShopReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RequestID string `gorm:"primaryKey;type:varbinary(36)" json:"requestId"`
	ProductID string `gorm:"type:varbinary(64);not null" json:"productId"`
	Version   string `gorm:"type:varbinary(64);not null" json:"version"`
	Response  []byte `gorm:"type:longblob;not null" json:"response"`
	CreatedAt int64  `gorm:"not null" json:"createdAt"`
}

func (GGOShopReceipt) TableName() string { return "ggo_shop_receipts" }

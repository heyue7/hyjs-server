package db

// P7RushReceipt 在角色行锁事务中绑定一次请求和原协议响应，防止购买/重置/开始重放。
type P7RushReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Domain    string `gorm:"primaryKey;type:varbinary(32)" json:"domain"`
	Timestamp int64  `gorm:"primaryKey" json:"timestamp"`
	Request   []byte `gorm:"type:longblob;not null" json:"-"`
	Response  []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P7RushReceipt) TableName() string { return "p7_rush_receipts" }

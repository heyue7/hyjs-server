package db

// OpenServerActivityReceipt 绑定完整原请求与二进制回包，多次购买可用新时间戳继续。
type OpenServerActivityReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Timestamp int64  `gorm:"primaryKey;autoIncrement:false" json:"timestamp"`
	Request   []byte `gorm:"type:longblob;not null" json:"request"`
	Response  []byte `gorm:"type:longblob;not null" json:"response"`
}

func (OpenServerActivityReceipt) TableName() string { return "open_server_activity_receipts" }

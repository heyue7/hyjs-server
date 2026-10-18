package db

// P8GatherReceipt 在玩家行锁事务内绑定原帧时间戳、请求与唯一业务响应。
type P8GatherReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Operation string `gorm:"primaryKey;size:32" json:"operation"`
	Timestamp int64  `gorm:"primaryKey;autoIncrement:false" json:"timestamp"`
	Request   []byte `gorm:"type:longblob;not null" json:"-"`
	Response  []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P8GatherReceipt) TableName() string { return "p8_gather_receipts" }

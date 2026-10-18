package db

// P4Receipt 冻结业务请求和原协议回包，真正重试沿用原结果，参数篡改拒绝。
type P4Receipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Domain    string `gorm:"primaryKey;type:varbinary(64)" json:"domain"`
	Timestamp int64  `gorm:"primaryKey;autoIncrement:false" json:"timestamp"`
	Request   []byte `gorm:"type:longblob;not null" json:"-"`
	Response  []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P4Receipt) TableName() string { return "p4_receipts" }

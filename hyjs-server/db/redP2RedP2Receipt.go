package db

// RedP2Receipt 保存 P2 原协议业务状态；运营未知规则在 Source 明示本地兼容。
type RedP2Receipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerID"`
	Operation string `gorm:"primaryKey;size:32" json:"operation"`
	Timestamp int64  `gorm:"primaryKey;autoIncrement:false" json:"timestamp"`
	Request   []byte `gorm:"type:longblob" json:"request"`
	Response  []byte `gorm:"type:longblob" json:"response"`
}

func (RedP2Receipt) TableName() string { return "p2_red_receipts" }

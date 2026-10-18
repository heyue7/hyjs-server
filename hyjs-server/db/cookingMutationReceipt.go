package db

// CookingMutationReceipt 将料理扣费与原始响应一起提交，重试不会重复累加永久属性。
type CookingMutationReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Opcode    int32  `gorm:"primaryKey;autoIncrement:false" json:"opcode"`
	Timestamp int64  `gorm:"not null" json:"timestamp"`
	Request   []byte `gorm:"type:longblob" json:"request"`
	Response  []byte `gorm:"type:longblob" json:"response"`
}

func (CookingMutationReceipt) TableName() string { return "cooking_mutation_receipts" }

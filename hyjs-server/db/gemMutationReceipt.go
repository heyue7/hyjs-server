package db

// GemMutationReceipt 将原始请求和结果与存档一起提交，避免网络重试重复扣费或随机奖励。
type GemMutationReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Opcode    int32  `gorm:"primaryKey;autoIncrement:false" json:"opcode"`
	Timestamp int64  `gorm:"not null" json:"timestamp"`
	Request   []byte `gorm:"type:longblob" json:"request"`
	Response  []byte `gorm:"type:longblob" json:"response"`
}

func (GemMutationReceipt) TableName() string { return "gem_mutation_receipts" }

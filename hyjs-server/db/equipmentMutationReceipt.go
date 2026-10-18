package db

// EquipmentMutationReceipt 保存原始包与响应，分解后的实例删除不影响重试。
type EquipmentMutationReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Opcode    int32  `gorm:"primaryKey;autoIncrement:false" json:"opcode"`
	Timestamp int64  `gorm:"not null" json:"timestamp"`
	Request   []byte `gorm:"type:longblob" json:"request"`
	Response  []byte `gorm:"type:longblob" json:"response"`
}

// TableName 指定装备操作回执表。
func (EquipmentMutationReceipt) TableName() string { return "equipment_mutation_receipts" }

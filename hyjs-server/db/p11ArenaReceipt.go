package db

// P11ArenaReceipt 绑定原请求及实际已冻结匹配批次，不将相同空请求误当新的队伍。
type P11ArenaReceipt struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false"`
	Operation   string `gorm:"primaryKey;type:varchar(32)"`
	Stamp       int64  `gorm:"primaryKey;autoIncrement:false"`
	RequestHash string `gorm:"type:varchar(64);not null"`
	Response    []byte `gorm:"type:longblob;not null"`
	BatchID     string `gorm:"type:varchar(64);not null"`
	CreatedAt   int64  `gorm:"not null"`
}

func (P11ArenaReceipt) TableName() string { return "p11_arena_receipts" }

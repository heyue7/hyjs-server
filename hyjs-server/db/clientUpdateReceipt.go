package db

// ClientUpdateReceipt 保存客户端更新的独立运行事实。
type ClientUpdateReceipt struct {
	BatchID   int64  `gorm:"column:batch_id;primaryKey;autoIncrement:false" json:"batchId"`
	StartupID string `gorm:"column:startup_id;primaryKey;size:64" json:"startupId"`
	Kind      string `gorm:"column:kind;primaryKey;size:24" json:"kind"`
	PlayerID  int64  `gorm:"column:player_id;not null;index" json:"playerId"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:false" json:"createdAt"`
}

func (ClientUpdateReceipt) TableName() string { return "client_update_receipts" }

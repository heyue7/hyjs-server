package db

// AdminBroadcastRecipient retains the exact preview audience and committed original delivery.
type AdminBroadcastRecipient struct {
	BatchID    string `gorm:"primaryKey;size:36" json:"batchId"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	PlayerName string `gorm:"size:191;not null" json:"playerName"`
	DeliveryID int64  `gorm:"not null" json:"deliveryId"`
}

func (AdminBroadcastRecipient) TableName() string { return "admin_broadcast_recipients" }

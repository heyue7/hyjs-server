package db

// P14Delivery 在本地事务内保留跨服快照投递与确认；目标离线不删除待确认载荷。
type P14Delivery struct {
	ID                  string `gorm:"primaryKey;type:char(64)" json:"id"`
	DestinationServerID int64  `gorm:"not null;index" json:"destinationServerId"`
	Payload             []byte `gorm:"type:longblob;not null" json:"-"`
	Confirmed           bool   `gorm:"not null;default:false;index" json:"confirmed"`
	AttemptCount        int64  `gorm:"not null;default:0" json:"attemptCount"`
	CreatedAt           int64  `gorm:"not null" json:"createdAt"`
	ConfirmedAt         int64  `gorm:"not null;default:0" json:"confirmedAt"`
}

func (P14Delivery) TableName() string { return "p14_deliveries" }

package db

// P14QualificationDelivery 不覆盖报名outbox，持久投递同一份真实资格证书直到对等服确认。
type P14QualificationDelivery struct {
	ID                  string `gorm:"type:char(64);primaryKey" json:"id"`
	DestinationServerID int64  `gorm:"not null;index" json:"destination_server_id"`
	SeasonIndex         int32  `gorm:"not null;index" json:"season_index"`
	Payload             []byte `gorm:"type:longblob;not null" json:"payload"`
	PayloadHash         string `gorm:"type:char(64);not null" json:"payload_hash"`
	Confirmed           bool   `gorm:"not null;default:false;index" json:"confirmed"`
	CreatedAt           int64  `gorm:"not null" json:"created_at"`
	ConfirmedAt         int64  `gorm:"not null;default:0" json:"confirmed_at"`
	AttemptCount        int64  `gorm:"not null;default:0" json:"attempt_count"`
}

func (P14QualificationDelivery) TableName() string { return "p14_qualification_deliveries" }

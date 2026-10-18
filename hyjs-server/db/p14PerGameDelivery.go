package db

// P14PerGameDelivery preserves independent real per-game sources without changing Game1 history.
type P14PerGameDelivery struct {
	ID                  string `gorm:"type:char(64);primaryKey" json:"id"`
	DestinationServerID int64  `gorm:"not null;index:idx_p14_per_game_delivery_pending" json:"destinationServerId"`
	Payload             []byte `gorm:"type:longblob;not null" json:"-"`
	PayloadHash         string `gorm:"type:char(64);not null" json:"payloadHash"`
	CreatedAt           int64  `gorm:"not null" json:"createdAt"`
	Confirmed           bool   `gorm:"not null;default:false;index:idx_p14_per_game_delivery_pending" json:"confirmed"`
	ConfirmedAt         int64  `gorm:"not null" json:"confirmedAt"`
	AttemptCount        int64  `gorm:"not null" json:"attemptCount"`
}

func (P14PerGameDelivery) TableName() string { return "p14_per_game_deliveries" }

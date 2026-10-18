package db

// LocalPlayPaymentTime projects a real order onto its owner's play calendar.
// Original order/confirmation timestamps and wallet money entries stay unchanged.
type LocalPlayPaymentTime struct {
	OrderID    string `gorm:"primaryKey;size:64" json:"order_id"`
	PlayerID   int64  `gorm:"not null;index" json:"player_id"`
	GameAt     int64  `gorm:"not null;index" json:"game_at"`
	RealAt     int64  `gorm:"not null" json:"real_at"`
	OffsetDays int64  `gorm:"not null" json:"offset_days"`
}

func (LocalPlayPaymentTime) TableName() string { return "local_play_payment_times" }

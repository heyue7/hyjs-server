package db

// ActivityRechargeCarry freezes accepted order identities before a calendar restore.
// Original payment dates and already claimed reward receipts remain immutable.
type ActivityRechargeCarry struct {
	InstanceID string `gorm:"primaryKey;size:64" json:"instance_id"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	RequestID  string `gorm:"size:128;not null" json:"request_id"`
	RealCutoff int64  `gorm:"not null" json:"real_cutoff"`
	OpeningAt  int64  `gorm:"not null" json:"opening_at"`
	Mode       string `gorm:"size:16;not null" json:"mode"`
	// Baseline freezes all pre-restore confirmed identities, including ineligible orders.
	Baseline []byte `gorm:"type:longblob;not null" json:"baseline"`
	Facts    []byte `gorm:"type:longblob;not null" json:"facts"`
	Digest   string `gorm:"size:64;not null" json:"digest"`
}

func (ActivityRechargeCarry) TableName() string { return "activity_recharge_carries" }

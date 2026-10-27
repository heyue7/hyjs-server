package db

// OpeningCalendarPaymentCarry freezes one opening ledger's paid qualification.
// Original orders, calendar projections and counter history remain immutable.
type OpeningCalendarPaymentCarry struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	ScopeID        string `gorm:"primaryKey;size:128" json:"scope_id"`
	RuleID         string `gorm:"size:64;not null" json:"rule_id"`
	RequestID      string `gorm:"size:128;not null" json:"request_id"`
	RealCutoff     int64  `gorm:"not null" json:"real_cutoff"`
	BeforeStartAt  int64  `gorm:"not null" json:"before_start_at"`
	BeforeEndAt    int64  `gorm:"not null" json:"before_end_at"`
	BeforeRewardAt int64  `gorm:"not null" json:"before_reward_at"`
	AfterStartAt   int64  `gorm:"not null" json:"after_start_at"`
	AfterEndAt     int64  `gorm:"not null" json:"after_end_at"`
	AfterRewardAt  int64  `gorm:"not null" json:"after_reward_at"`
	Facts          []byte `gorm:"type:longblob;not null" json:"facts"`
	Baseline       []byte `gorm:"type:longblob;not null" json:"baseline"`
	Digest         string `gorm:"size:64;not null" json:"digest"`
}

func (OpeningCalendarPaymentCarry) TableName() string { return "opening_calendar_payment_carries" }

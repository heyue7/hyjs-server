package db

// OpeningCalendarCarry preserves the original dated-counter image for one
// stable opening ledger. LoginDays records only real post-restore logins.
type OpeningCalendarCarry struct {
	PlayerID         int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Scope            string `gorm:"primaryKey;size:80" json:"scope"`
	RequestID        string `gorm:"size:128;not null" json:"requestId"`
	BeforeStartAt    int64  `gorm:"not null" json:"beforeStartAt"`
	BeforeEndAt      int64  `gorm:"not null" json:"beforeEndAt"`
	BeforeClaimEndAt int64  `gorm:"not null" json:"beforeClaimEndAt"`
	AfterStartAt     int64  `gorm:"not null" json:"afterStartAt"`
	AfterEndAt       int64  `gorm:"not null" json:"afterEndAt"`
	AfterClaimEndAt  int64  `gorm:"not null" json:"afterClaimEndAt"`
	OldNow           int64  `gorm:"not null" json:"oldNow"`
	RealCutoff       int64  `gorm:"not null" json:"realCutoff"`
	Baseline         []byte `gorm:"type:longblob;not null" json:"baseline"`
	LoginDays        []byte `gorm:"type:longblob;not null" json:"loginDays"`
	Digest           string `gorm:"size:64;not null" json:"digest"`
}

func (OpeningCalendarCarry) TableName() string { return "opening_calendar_carries" }

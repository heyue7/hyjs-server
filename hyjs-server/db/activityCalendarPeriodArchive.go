package db

// ActivityCalendarPeriodArchive preserves the complete original settlement before reopening.
// One archive per stable period prevents a later request from consuming new settlement history.
type ActivityCalendarPeriodArchive struct {
	InstanceID       string `gorm:"type:varbinary(64);primaryKey" json:"instanceId"`
	RequestID        string `gorm:"size:128;not null" json:"requestId"`
	SettledAt        int64  `gorm:"not null" json:"settledAt"`
	Snapshot         []byte `gorm:"type:longblob;not null" json:"snapshot"`
	BeforeStartAt    int64  `gorm:"not null" json:"beforeStartAt"`
	BeforeEndAt      int64  `gorm:"not null" json:"beforeEndAt"`
	BeforeClaimEndAt int64  `gorm:"not null" json:"beforeClaimEndAt"`
	AfterStartAt     int64  `gorm:"not null" json:"afterStartAt"`
	AfterEndAt       int64  `gorm:"not null" json:"afterEndAt"`
	AfterClaimEndAt  int64  `gorm:"not null" json:"afterClaimEndAt"`
	Digest           string `gorm:"size:64;not null" json:"digest"`
}

func (ActivityCalendarPeriodArchive) TableName() string { return "activity_calendar_period_archives" }

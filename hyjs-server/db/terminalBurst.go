package db

// TerminalBurst persists the terminal contract without rewriting historical wallet rows.
type TerminalBurst struct {
	AccountID   int64  `gorm:"primaryKey;autoIncrement:false;uniqueIndex:terminal_burst_day" json:"accountID"`
	RequestID   string `gorm:"size:128;primaryKey" json:"requestID"`
	Day         string `gorm:"size:10;not null;uniqueIndex:terminal_burst_day" json:"day"`
	AwardPoints int64  `gorm:"not null" json:"awardPoints"`
}

func (TerminalBurst) TableName() string { return "terminal_bursts" }

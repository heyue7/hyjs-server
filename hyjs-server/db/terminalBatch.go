package db

// TerminalBatch persists the terminal contract without rewriting historical wallet rows.
type TerminalBatch struct {
	ID         string `gorm:"size:96;primaryKey" json:"iD"`
	ServerID   int64  `gorm:"not null;index" json:"serverID"`
	RequestID  string `gorm:"size:128;not null" json:"requestID"`
	BodyHash   string `gorm:"size:64;not null" json:"bodyHash"`
	Points     int64  `gorm:"not null" json:"points"`
	Reason     string `gorm:"size:255;not null" json:"reason"`
	Kind       string `gorm:"size:24;not null" json:"kind"`
	Recipients int64  `gorm:"not null" json:"recipients"`
	Credited   int64  `gorm:"not null" json:"credited"`
	CreatedAt  int64  `gorm:"not null;autoCreateTime:false" json:"createdAt"`
	Status     string `gorm:"size:24;not null" json:"status"`
}

func (TerminalBatch) TableName() string { return "terminal_batches" }

package db

// TerminalRoleBurst persists the terminal contract without rewriting historical wallet rows.
type TerminalRoleBurst struct {
	AccountID   int64  `gorm:"not null;index" json:"accountId"`
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false;uniqueIndex:terminal_role_bursts_day" json:"serverId"`
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false;uniqueIndex:terminal_role_bursts_day" json:"playerId"`
	RequestID   string `gorm:"size:128;primaryKey" json:"requestID"`
	Day         string `gorm:"size:10;not null;uniqueIndex:terminal_role_bursts_day" json:"day"`
	AwardPoints int64  `gorm:"not null" json:"awardPoints"`
}

func (TerminalRoleBurst) TableName() string { return "terminal_role_bursts" }

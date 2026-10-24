package db

// TerminalRoleState persists the terminal contract without rewriting historical wallet rows.
type TerminalRoleState struct {
	AccountID         int64 `gorm:"not null;index" json:"accountId"`
	ServerID          int64 `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	PlayerID          int64 `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	SeenRevision      int64 `gorm:"not null" json:"seenRevision"`
	DeliveredRevision int64 `gorm:"not null" json:"deliveredRevision"`
}

func (TerminalRoleState) TableName() string { return "terminal_role_states" }

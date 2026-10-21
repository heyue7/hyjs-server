package db

// TerminalAccountState persists the terminal contract without rewriting historical wallet rows.
type TerminalAccountState struct {
	AccountID         int64 `gorm:"primaryKey;autoIncrement:false" json:"accountID"`
	SeenRevision      int64 `gorm:"not null" json:"seenRevision"`
	DeliveredRevision int64 `gorm:"not null" json:"deliveredRevision"`
}

func (TerminalAccountState) TableName() string { return "terminal_account_states" }

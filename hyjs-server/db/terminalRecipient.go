package db

// TerminalRecipient persists the terminal contract without rewriting historical wallet rows.
type TerminalRecipient struct {
	BatchID   string `gorm:"size:96;primaryKey" json:"batchID"`
	AccountID int64  `gorm:"primaryKey;autoIncrement:false" json:"accountID"`
	PlayerID  int64  `gorm:"not null" json:"playerID"`
	Label     string `gorm:"size:255;not null" json:"label"`
	ReasonID  string `gorm:"size:64;not null" json:"reasonID"`
	Applied   bool   `gorm:"not null" json:"applied"`
}

func (TerminalRecipient) TableName() string { return "terminal_recipients" }

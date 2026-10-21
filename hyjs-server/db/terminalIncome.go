package db

// TerminalIncome persists the terminal contract without rewriting historical wallet rows.
type TerminalIncome struct {
	AccountID   int64  `gorm:"primaryKey;autoIncrement:false" json:"accountID"`
	ID          string `gorm:"size:128;primaryKey" json:"iD"`
	Label       string `gorm:"size:255;not null" json:"label"`
	AmountCents int64  `gorm:"not null" json:"amountCents"`
	CreatedAt   int64  `gorm:"not null;autoCreateTime:false" json:"createdAt"`
	Revision    int64  `gorm:"not null;index" json:"revision"`
	Source      string `gorm:"size:24;not null" json:"source"`
}

func (TerminalIncome) TableName() string { return "terminal_incomes" }

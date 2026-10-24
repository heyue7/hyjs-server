package db

// TerminalRoleIncome persists the terminal contract without rewriting historical wallet rows.
type TerminalRoleIncome struct {
	AccountID   int64  `gorm:"not null;index" json:"accountId"`
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false;index:terminal_role_income_revision,priority:1" json:"serverId"`
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false;index:terminal_role_income_revision,priority:2" json:"playerId"`
	ID          string `gorm:"size:128;primaryKey" json:"id"`
	Label       string `gorm:"size:255;not null" json:"label"`
	AmountCents int64  `gorm:"not null" json:"amountCents"`
	CreatedAt   int64  `gorm:"not null;autoCreateTime:false" json:"createdAt"`
	Revision    int64  `gorm:"not null;index:terminal_role_income_revision,priority:3" json:"revision"`
	Source      string `gorm:"size:24;not null" json:"source"`
}

func (TerminalRoleIncome) TableName() string { return "terminal_role_incomes" }

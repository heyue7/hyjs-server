package db

// P7RushScopedCalculation binds one real paid P7 fight to its immutable P6 source
// and complete original automatic record. Diagnostic fights never create a row.
type P7RushScopedCalculation struct {
	FightID      string `gorm:"primaryKey;type:varbinary(64)" json:"fightId"`
	PlayerID     int64  `gorm:"not null;index" json:"playerId"`
	P6RecordID   string `gorm:"type:char(64);not null" json:"p6RecordId"`
	Series       []byte `gorm:"type:longblob;not null" json:"-"`
	SeriesHash   string `gorm:"type:char(64);not null" json:"seriesHash"`
	Terminal     []byte `gorm:"type:longblob;not null" json:"-"`
	TerminalHash string `gorm:"type:char(64);not null" json:"terminalHash"`
	StartHash    string `gorm:"type:char(64);not null" json:"startHash"`
	ScopeProof   string `gorm:"type:char(64);not null" json:"scopeProof"`
	CreatedAt    int64  `gorm:"autoCreateTime:false;not null" json:"createdAt"`
}

func (P7RushScopedCalculation) TableName() string { return "p7_rush_scoped_calculations" }

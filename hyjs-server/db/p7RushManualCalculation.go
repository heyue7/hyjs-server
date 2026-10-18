package db

// P7RushManualCalculation freezes permission before payment returns. Pending
// rows have only an independently certified automatic preview. The actual
// terminal is signed once after a whole live-driver recomputation at End.
type P7RushManualCalculation struct {
	FightID                        string `gorm:"primaryKey;type:varbinary(64)" json:"fightId"`
	PlayerID                       int64  `gorm:"not null;index" json:"playerId"`
	P6RecordID                     string `gorm:"type:char(64);not null" json:"p6RecordId"`
	DriverPolicy                   string `gorm:"type:varchar(255);not null" json:"driverPolicy"`
	ScopeProof                     string `gorm:"type:char(64);not null" json:"scopeProof"`
	Status                         int64  `gorm:"not null;default:0" json:"status"`
	Series                         []byte `gorm:"type:longblob;not null" json:"-"`
	SeriesHash                     string `gorm:"type:char(64);not null" json:"seriesHash"`
	StartHash                      string `gorm:"type:char(64);not null" json:"startHash"`
	Preview                        []byte `gorm:"type:longblob;not null" json:"-"`
	PreviewHash                    string `gorm:"type:char(64);not null" json:"previewHash"`
	Terminal                       []byte `gorm:"type:longblob" json:"-"`
	TerminalHash                   string `gorm:"type:char(64);not null;default:''" json:"terminalHash"`
	DerivedCalculationSourceSHA256 string `gorm:"type:char(64);not null;default:''" json:"derivedCalculationSourceSHA256"`
	EndRequestHash                 string `gorm:"type:char(64);not null;default:''" json:"endRequestHash"`
	CreatedAt                      int64  `gorm:"not null;autoCreateTime:false" json:"createdAt"`
	AcceptedAt                     int64  `gorm:"not null;default:0" json:"acceptedAt"`
}

func (P7RushManualCalculation) TableName() string { return "p7_rush_manual_calculations" }

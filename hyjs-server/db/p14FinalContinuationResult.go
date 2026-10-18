package db

// P14FinalContinuationResult is one accepted canonical result, never a viewer's claim.
type P14FinalContinuationResult struct {
	SeriesID       string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	GameIndex      int32  `gorm:"primaryKey;autoIncrement:false" json:"gameIndex"`
	OfferHash      string `gorm:"type:char(64);not null" json:"offerHash"`
	InputID        string `gorm:"type:char(64);not null" json:"inputId"`
	InputHash      string `gorm:"type:char(64);not null" json:"inputHash"`
	ProvenanceHash string `gorm:"type:char(64);not null" json:"provenanceHash"`
	P6RecordID     string `gorm:"type:char(64);not null" json:"p6RecordId"`
	ScopeProof     string `gorm:"type:char(64);not null" json:"scopeProof"`
	WinnerCamp     int32  `gorm:"not null" json:"winnerCamp"`
	WinnerServerID int64  `gorm:"not null" json:"winnerServerId"`
	WinnerPlayerID int64  `gorm:"not null" json:"winnerPlayerId"`
	Terminal       []byte `gorm:"type:longblob;not null" json:"-"`
	TerminalHash   string `gorm:"type:char(64);not null" json:"terminalHash"`
	CreatedAt      int64  `gorm:"not null" json:"createdAt"`
	SourceHash     string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14FinalContinuationResult) TableName() string { return "p14_final_continuation_results" }

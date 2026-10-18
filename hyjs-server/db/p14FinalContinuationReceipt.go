package db

// P14FinalContinuationReceipt stores the authenticated remote terminal and its frozen
// input. It never creates a local P6BattleRecord for the remote owner's PID.
type P14FinalContinuationReceipt struct {
	SeriesID         string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	GameIndex        int32  `gorm:"primaryKey;autoIncrement:false" json:"gameIndex"`
	DeliveryID       string `gorm:"type:char(64);uniqueIndex;not null" json:"deliveryId"`
	SourceServerID   int64  `gorm:"not null" json:"sourceServerId"`
	PayloadHash      string `gorm:"type:char(64);not null" json:"payloadHash"`
	SourceResult     []byte `gorm:"type:longblob;not null" json:"-"`
	SourceResultHash string `gorm:"type:char(64);not null" json:"sourceResultHash"`
	Input            []byte `gorm:"type:longblob;not null" json:"-"`
	InputHash        string `gorm:"type:char(64);not null" json:"inputHash"`
	Terminal         []byte `gorm:"type:longblob;not null" json:"-"`
	TerminalHash     string `gorm:"type:char(64);not null" json:"terminalHash"`
	OfferHash        string `gorm:"type:char(64);not null" json:"offerHash"`
	WinnerServerID   int64  `gorm:"not null" json:"winnerServerId"`
	WinnerPlayerID   int64  `gorm:"not null" json:"winnerPlayerId"`
	ReceivedAt       int64  `gorm:"not null" json:"receivedAt"`
	SourceHash       string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14FinalContinuationReceipt) TableName() string { return "p14_final_continuation_receipts" }

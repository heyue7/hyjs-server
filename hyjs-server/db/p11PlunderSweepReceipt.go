package db

// P11PlunderSweepReceipt owns one original56009 packet and its actual child
// settlements. Child receipt keys stay internal; HTTP timestamps remain positive.
type P11PlunderSweepReceipt struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Stamp          int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	RequestHash    string `gorm:"type:varchar(64);not null" json:"requestHash"`
	FragmentID     string `gorm:"type:varchar(64);not null" json:"fragmentId"`
	RequestedCount int32  `gorm:"not null" json:"requestedCount"`
	AcceptedCount  int32  `gorm:"not null" json:"acceptedCount"`
	Plan           []byte `gorm:"type:longblob;not null" json:"plan"`
	SourceHash     string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	Response       []byte `gorm:"type:longblob;not null" json:"response"`
	ResponseHash   string `gorm:"type:varchar(64);not null" json:"responseHash"`
	Source         string `gorm:"type:varchar(191);not null" json:"source"`
	CreatedAt      int64  `gorm:"not null" json:"createdAt"`
	FinishedAt     int64  `gorm:"not null" json:"finishedAt"`
}

func (P11PlunderSweepReceipt) TableName() string { return "p11_plunder_sweep_receipts" }

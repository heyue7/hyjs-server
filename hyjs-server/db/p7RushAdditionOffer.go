package db

// P7RushAdditionOffer 按轮次冻结首层来源与三选项，真实结算后只允许支付一次。
type P7RushAdditionOffer struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RunID          int64  `gorm:"primaryKey;autoIncrement:false" json:"runId"`
	FirstFightID   string `gorm:"type:varbinary(64);not null" json:"firstFightId"`
	FirstStartHash string `gorm:"size:64;not null" json:"firstStartHash"`
	StartedAt      int64  `gorm:"not null" json:"startedAt"`
	Policy         []byte `gorm:"type:longblob;not null" json:"policy"`
	PolicyHash     string `gorm:"size:64;not null" json:"policyHash"`
	StageRules     []byte `gorm:"type:longblob;not null" json:"stageRules"`
	StageHash      string `gorm:"size:64;not null" json:"stageHash"`
	Selected       string `gorm:"size:64;not null" json:"selected"`
	SpentStars     int32  `gorm:"not null" json:"spentStars"`
	SelectedAt     int64  `gorm:"not null" json:"selectedAt"`
	SelectionStamp int64  `gorm:"not null" json:"selectionStamp"`
	AcceptanceHash string `gorm:"size:64;not null" json:"acceptanceHash"`
}

func (P7RushAdditionOffer) TableName() string { return "p7_rush_addition_offers" }

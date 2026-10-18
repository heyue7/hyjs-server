package db

// P14PairGuess preserves one actual prepublished pair source; it carries no fabricated result.
type P14PairGuess struct {
	SeasonIndex    int32  `gorm:"primaryKey;autoIncrement:false" json:"seasonIndex"`
	PairHash       string `gorm:"primaryKey;type:char(64)" json:"pairHash"`
	OwnerServerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"ownerServerId"`
	OwnerPlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"ownerPlayerId"`
	TargetServerID int64  `gorm:"not null" json:"targetServerId"`
	TargetPlayerID int64  `gorm:"not null" json:"targetPlayerId"`
	OfferHash      string `gorm:"type:char(64);not null" json:"offerHash"`
	CostCoin       int64  `gorm:"not null" json:"costCoin"`
	CoinBefore     int64  `gorm:"not null" json:"coinBefore"`
	CoinAfter      int64  `gorm:"not null" json:"coinAfter"`
	CapturedAt     int64  `gorm:"not null" json:"capturedAt"`
	SourceHash     string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14PairGuess) TableName() string { return "p14_pair_guesses" }

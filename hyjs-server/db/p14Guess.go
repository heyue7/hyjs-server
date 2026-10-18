package db

// P14Guess records one genuine debit and its joint target, without inventing a final winner.
type P14Guess struct {
	SeasonIndex    int32  `gorm:"primaryKey;autoIncrement:false" json:"seasonIndex"`
	GuessType      int32  `gorm:"primaryKey;autoIncrement:false" json:"guessType"`
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

func (P14Guess) TableName() string { return "p14_guesses" }

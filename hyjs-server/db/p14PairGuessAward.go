package db

// P14PairGuessAward binds one prepaid bet to its complete certified series and original reward-center delivery.
type P14PairGuessAward struct {
	SeasonIndex        int32  `gorm:"primaryKey;autoIncrement:false" json:"seasonIndex"`
	PairHash           string `gorm:"type:char(64);primaryKey" json:"pairHash"`
	OwnerServerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"ownerServerId"`
	OwnerPlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"ownerPlayerId"`
	GuessHash          string `gorm:"type:char(64);not null" json:"guessHash"`
	OfferHash          string `gorm:"type:char(64);not null" json:"offerHash"`
	SeriesID           string `gorm:"type:char(64);not null" json:"seriesId"`
	SeriesManifestHash string `gorm:"type:char(64);not null" json:"seriesManifestHash"`
	SeriesClosureHash  string `gorm:"type:char(64);not null" json:"seriesClosureHash"`
	WinnerServerID     int64  `gorm:"not null" json:"winnerServerId"`
	WinnerPlayerID     int64  `gorm:"not null" json:"winnerPlayerId"`
	SelectedWon        bool   `gorm:"not null" json:"selectedWon"`
	Coin               int64  `gorm:"not null" json:"coin"`
	Diamond            int64  `gorm:"not null" json:"diamond"`
	RewardID           int64  `gorm:"not null;uniqueIndex" json:"rewardId"`
	RewardHash         string `gorm:"type:char(64);not null" json:"rewardHash"`
	Source             string `gorm:"type:text;not null" json:"source"`
	CreatedAt          int64  `gorm:"not null" json:"createdAt"`
	SourceHash         string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14PairGuessAward) TableName() string { return "p14_pair_guess_awards" }

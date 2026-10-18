package db

// P14FinalSeries binds real series scheduling to its frozen source, without presumed battle outcomes.
type P14FinalSeries struct {
	ID             string `gorm:"type:char(64);primaryKey" json:"id"`
	SeasonIndex    int32  `gorm:"not null;uniqueIndex:idx_p14_series_pair" json:"seasonIndex"`
	RoundStage     int32  `gorm:"not null;uniqueIndex:idx_p14_series_pair" json:"roundStage"`
	GroupID        string `gorm:"type:char(1);not null;uniqueIndex:idx_p14_series_pair" json:"groupId"`
	PairIndex      int32  `gorm:"not null;uniqueIndex:idx_p14_series_pair" json:"pairIndex"`
	PairHash       string `gorm:"type:char(64);not null" json:"pairHash"`
	GameOneID      string `gorm:"type:char(64);not null" json:"gameOneId"`
	Manifest       []byte `gorm:"type:longblob;not null" json:"manifest"`
	ManifestHash   string `gorm:"type:char(64);not null" json:"manifestHash"`
	State          int32  `gorm:"not null" json:"state"`
	CurrentGame    int32  `gorm:"not null" json:"currentGame"`
	LeftWins       int32  `gorm:"not null" json:"leftWins"`
	RightWins      int32  `gorm:"not null" json:"rightWins"`
	WinnerServerID int64  `gorm:"not null" json:"winnerServerId"`
	WinnerPlayerID int64  `gorm:"not null" json:"winnerPlayerId"`
	CreatedAt      int64  `gorm:"not null" json:"createdAt"`
	UpdatedAt      int64  `gorm:"not null" json:"updatedAt"`
}

func (P14FinalSeries) TableName() string { return "p14_final_series" }

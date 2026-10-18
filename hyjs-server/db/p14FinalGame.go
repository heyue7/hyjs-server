package db

// P14FinalGame freezes only a real first-pair input. It contains no winner or score.
type P14FinalGame struct {
	ID             string `gorm:"type:char(64);primaryKey" json:"id"`
	SeasonIndex    int32  `gorm:"not null;uniqueIndex:idx_p14_final_game_pair" json:"seasonIndex"`
	RoundStage     int32  `gorm:"not null;uniqueIndex:idx_p14_final_game_pair" json:"roundStage"`
	GroupID        string `gorm:"type:char(1);not null;uniqueIndex:idx_p14_final_game_pair" json:"groupId"`
	PairIndex      int32  `gorm:"not null;uniqueIndex:idx_p14_final_game_pair" json:"pairIndex"`
	GameIndex      int32  `gorm:"not null;uniqueIndex:idx_p14_final_game_pair" json:"gameIndex"`
	Replay         []byte `gorm:"type:longblob;not null" json:"-"`
	ReplayHash     string `gorm:"type:char(64);not null" json:"replayHash"`
	Provenance     []byte `gorm:"type:longblob;not null" json:"-"`
	ProvenanceHash string `gorm:"type:char(64);not null" json:"provenanceHash"`
	CreatedAt      int64  `gorm:"not null" json:"createdAt"`
}

func (P14FinalGame) TableName() string { return "p14_final_games" }

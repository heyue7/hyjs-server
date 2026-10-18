package db

// P14PairGuessOffer preserves one actual prepublished pair source; it carries no fabricated result.
type P14PairGuessOffer struct {
	SeasonIndex       int32  `gorm:"primaryKey;autoIncrement:false" json:"seasonIndex"`
	PairHash          string `gorm:"primaryKey;type:char(64)" json:"pairHash"`
	BattleType        int32  `gorm:"not null" json:"battleType"`
	Pair              []byte `gorm:"type:longblob;not null" json:"pair"`
	DrawHash          string `gorm:"type:char(64);not null" json:"drawHash"`
	QualificationHash string `gorm:"type:char(64);not null" json:"qualificationHash"`
	Version           string `gorm:"type:varchar(128);not null" json:"version"`
	Source            string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt       int64  `gorm:"not null" json:"publishedAt"`
	OpenAt            int64  `gorm:"not null" json:"openAt"`
	CloseAt           int64  `gorm:"not null" json:"closeAt"`
	CostCoin          int64  `gorm:"not null" json:"costCoin"`
	RevertCoin        int64  `gorm:"not null" json:"revertCoin"`
	SuccessCoin       int64  `gorm:"not null" json:"successCoin"`
	ExtraDiamond      int64  `gorm:"not null" json:"extraDiamond"`
	Settlement        string `gorm:"type:varchar(512);not null" json:"settlement"`
	SourceHash        string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14PairGuessOffer) TableName() string { return "p14_pair_guess_offers" }

package db

// P14GuessOffer freezes original table numbers and real common candidates before payment.
type P14GuessOffer struct {
	SeasonIndex       int32  `gorm:"primaryKey;autoIncrement:false" json:"seasonIndex"`
	GuessType         int32  `gorm:"primaryKey;autoIncrement:false" json:"guessType"`
	Version           string `gorm:"type:varchar(128);not null" json:"version"`
	Source            string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt       int64  `gorm:"not null" json:"publishedAt"`
	OpenAt            int64  `gorm:"not null" json:"openAt"`
	CloseAt           int64  `gorm:"not null" json:"closeAt"`
	QualificationHash string `gorm:"type:char(64);not null" json:"qualificationHash"`
	DrawHash          string `gorm:"type:char(64);not null" json:"drawHash"`
	Rules             []byte `gorm:"type:longblob;not null" json:"rules"`
	RulesHash         string `gorm:"type:char(64);not null" json:"rulesHash"`
	SourceHash        string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14GuessOffer) TableName() string { return "p14_guess_offers" }

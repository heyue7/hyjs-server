package db

// P14FinalFormationOffer binds one real final qualification and original calendar to a capture lock.
type P14FinalFormationOffer struct {
	SeasonIndex       int32  `gorm:"primaryKey" json:"seasonIndex"`
	RoundStage        int32  `gorm:"primaryKey;autoIncrement:false" json:"roundStage"`
	Version           string `gorm:"type:varchar(128);not null" json:"version"`
	Source            string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt       int64  `gorm:"not null" json:"publishedAt"`
	QualificationHash string `gorm:"type:char(64);not null" json:"qualificationHash"`
	StageStart        int64  `gorm:"not null" json:"stageStart"`
	FightStart        int64  `gorm:"not null" json:"fightStart"`
	FightEnd          int64  `gorm:"not null" json:"fightEnd"`
	LockAt            int64  `gorm:"not null" json:"lockAt"`
	SourceHash        string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14FinalFormationOffer) TableName() string { return "p14_final_formation_offers" }

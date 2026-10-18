package db

// P14FinalFormation preserves the actual qualified player capture for one original final round.
type P14FinalFormation struct {
	SeasonIndex       int32  `gorm:"primaryKey" json:"seasonIndex"`
	RoundStage        int32  `gorm:"primaryKey;autoIncrement:false" json:"roundStage"`
	ServerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Revision          int64  `gorm:"not null" json:"revision"`
	CapturedAt        int64  `gorm:"not null" json:"capturedAt"`
	FightPower        int32  `gorm:"not null" json:"fightPower"`
	Snapshot          []byte `gorm:"type:longblob;not null" json:"-"`
	SnapshotHash      string `gorm:"type:char(64);not null" json:"snapshotHash"`
	Battle            []byte `gorm:"type:longblob;not null" json:"-"`
	ClientVersion     string `gorm:"type:varchar(128);not null" json:"clientVersion"`
	EngineVersion     string `gorm:"type:varchar(128);not null" json:"engineVersion"`
	ConfigurationHash string `gorm:"type:char(64);not null" json:"configurationHash"`
	QualificationHash string `gorm:"type:char(64);not null" json:"qualificationHash"`
	OfferHash         string `gorm:"type:char(64);not null" json:"offerHash"`
	Source            string `gorm:"type:varchar(1024);not null" json:"source"`
	SourceHash        string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14FinalFormation) TableName() string { return "p14_final_formations" }

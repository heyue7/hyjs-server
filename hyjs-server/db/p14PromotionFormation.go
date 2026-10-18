package db

// P14PromotionFormation stores a real qualified stage7 upload independently of
// the immutable registration and audition roster. Both realms retain joint IDs.
type P14PromotionFormation struct {
	SeasonIndex       int32  `gorm:"primaryKey" json:"seasonIndex"`
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
	StageStart        int64  `gorm:"not null" json:"stageStart"`
	StageEnd          int64  `gorm:"not null" json:"stageEnd"`
	Source            string `gorm:"type:varchar(1024);not null" json:"source"`
	SourceHash        string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14PromotionFormation) TableName() string { return "p14_promotion_formations" }

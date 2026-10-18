package db

// P7ExpeditionMission 冻结发布时任务报价，接受与领奖分别保留原回执。
type P7ExpeditionMission struct {
	Serial     string `gorm:"primaryKey;type:varbinary(64)" json:"serial"`
	PlayerID   int64  `gorm:"not null;index" json:"playerId"`
	MissionID  string `gorm:"type:varchar(64);not null" json:"missionId"`
	MainHeroID string `gorm:"type:varchar(64);not null" json:"mainHeroId"`
	State      int32  `gorm:"not null" json:"state"`
	StartAt    int64  `gorm:"not null" json:"startAt"`
	EndAt      int64  `gorm:"not null" json:"endAt"`
	Quote      []byte `gorm:"type:longblob;not null" json:"-"`
	QuoteHash  string `gorm:"type:char(64);not null" json:"quoteHash"`
	Heroes     []byte `gorm:"type:longblob" json:"-"`
	Result     []byte `gorm:"type:longblob" json:"-"`
}

func (P7ExpeditionMission) TableName() string { return "p7_expedition_missions" }

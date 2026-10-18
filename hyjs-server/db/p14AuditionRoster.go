package db

// P14AuditionRoster 保存本服报名截止前的真实阵容；晋级与淘汰需要后续权威比赛结果。
type P14AuditionRoster struct {
	SeasonIndex       int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Revision          int64  `gorm:"not null" json:"revision"`
	CapturedAt        int64  `gorm:"not null" json:"captured_at"`
	FightPower        int32  `gorm:"not null" json:"fight_power"`
	Snapshot          []byte `gorm:"type:longblob;not null" json:"snapshot"`
	Battle            []byte `gorm:"type:longblob;not null" json:"battle"`
	SnapshotHash      string `gorm:"type:char(64);not null" json:"snapshot_hash"`
	ClientVersion     string `gorm:"type:varchar(128);not null" json:"client_version"`
	EngineVersion     string `gorm:"type:varchar(128);not null" json:"engine_version"`
	ConfigurationHash string `gorm:"type:char(64);not null" json:"configuration_hash"`
	SourceHash        string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P14AuditionRoster) TableName() string { return "p14_audition_rosters" }

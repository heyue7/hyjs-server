package db

// P14Member 以原赛季、可信原服号和本地角色号联合标识真实上传阵容。
type P14Member struct {
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
}

func (P14Member) TableName() string { return "p14_members" }

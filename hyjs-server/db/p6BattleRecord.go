package db

// P6BattleRecord 冻结归属、双方原实例、配置、随机种子、操作和结果；来源内唯一。
type P6BattleRecord struct {
	ID                string `gorm:"primaryKey;size:64" json:"id"`
	PlayerID          int64  `gorm:"not null;uniqueIndex:p6_source,priority:1;index;index:p6_playback_alias,priority:1" json:"playerId"`
	Source            string `gorm:"size:32;not null;uniqueIndex:p6_source,priority:2" json:"source"`
	SourceFight       string `gorm:"size:128;not null;uniqueIndex:p6_source,priority:3;index:p6_playback_alias,priority:2" json:"sourceFight"`
	Status            string `gorm:"size:16;not null" json:"status"`
	Seed              int64  `json:"seed"`
	ConfigurationHash string `gorm:"size:64;not null" json:"configurationHash"`
	EngineVersion     string `gorm:"size:64" json:"engineVersion"`
	ClientVersion     string `gorm:"size:64" json:"clientVersion"`
	Input             []byte `gorm:"type:longblob;not null" json:"-"`
	Operates          []byte `gorm:"type:longblob" json:"-"`
	Outcome           []byte `gorm:"type:longblob" json:"-"`
	InputHash         string `gorm:"size:64;not null" json:"inputHash"`
	ReplayHash        string `gorm:"size:64" json:"replayHash"`
	OutcomeHash       string `gorm:"size:64" json:"outcomeHash"`
	ClientResult      int32  `json:"clientResult"`
	ServerResult      int32  `json:"serverResult"`
	Authoritative     bool   `json:"authoritative"`
	CreatedAt         int64  `gorm:"autoCreateTime:false" json:"createdAt"`
	ExpiresAt         int64  `json:"expiresAt"`
	FinishedAt        int64  `json:"finishedAt"`
}

func (P6BattleRecord) TableName() string { return "p6_battle_records" }

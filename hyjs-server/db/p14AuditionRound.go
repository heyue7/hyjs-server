package db

// P14AuditionRound 固定当轮真实存活成员与随机配对，奇数未配对者不产生轮空胜负。
type P14AuditionRound struct {
	SeasonIndex  int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	RoundIndex   int64  `gorm:"primaryKey;autoIncrement:false" json:"round_index"`
	BoundaryHash string `gorm:"size:64;not null" json:"boundary_hash"`
	Participants []byte `gorm:"type:longblob;not null" json:"participants"`
	MatchHash    string `gorm:"size:64;not null" json:"match_hash"`
	CreatedAt    int64  `gorm:"autoCreateTime:false;not null" json:"created_at"`
	SourceHash   string `gorm:"size:64;not null" json:"source_hash"`
}

func (P14AuditionRound) TableName() string { return "p14_audition_rounds" }

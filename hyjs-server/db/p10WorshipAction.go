package db

// P10WorshipAction 保存原资格榜单中的真实目标/名次与一次膜拜奖励事实。
type P10WorshipAction struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day          string `gorm:"primaryKey;size:10" json:"day"`
	TargetID     int64  `gorm:"primaryKey;autoIncrement:false" json:"target_id"`
	RankType     int32  `gorm:"not null" json:"rank_type"`
	RankPosition int64  `gorm:"not null" json:"rank_position"`
	TargetInfo   []byte `gorm:"type:longblob;not null" json:"target_info"`
	DailyHash    string `gorm:"size:64;not null" json:"daily_hash"`
	CreatedAt    int64  `gorm:"autoCreateTime:false;not null" json:"created_at"`
	SourceHash   string `gorm:"size:64;not null" json:"source_hash"`
}

func (P10WorshipAction) TableName() string { return "p10_worship_actions" }

package db

// DailyCopyFight 将开始请求、阵容和预抽奖励冻结；结算回执独立保存以跨日重试。
type DailyCopyFight struct {
	FightID    string `gorm:"primaryKey;size:64" json:"fight_id"`
	PlayerID   int64  `gorm:"not null;index:idx_daily_copy_active,priority:1;uniqueIndex:idx_daily_copy_start,priority:1" json:"player_id"`
	StartStamp int64  `gorm:"not null;uniqueIndex:idx_daily_copy_start,priority:2" json:"start_stamp"`
	Day        string `gorm:"size:10;not null;index:idx_daily_copy_active,priority:2" json:"day"`
	Type       string `gorm:"size:64;not null" json:"type"`
	ChapterID  string `gorm:"size:64;not null" json:"chapter_id"`
	Stamina    int32  `gorm:"not null" json:"stamina"`
	WinStars   int32  `gorm:"not null" json:"win_stars"`
	Reward     string `gorm:"type:text;not null" json:"reward"`
	Start      []byte `gorm:"column:start_snapshot;type:longblob;not null" json:"start"`
	Settled    bool   `gorm:"not null;index:idx_daily_copy_active,priority:3" json:"settled"`
	EndRequest []byte `gorm:"type:longblob" json:"end_request"`
	Response   []byte `gorm:"type:longblob" json:"response"`
}

func (DailyCopyFight) TableName() string { return "daily_copy_fights" }

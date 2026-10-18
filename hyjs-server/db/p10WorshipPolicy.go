package db

// P10WorshipPolicy 单独控制原排行榜膜拜，不依赖世界/好友/公会开关。
type P10WorshipPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null;default:0" json:"enabled"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P10WorshipPolicy) TableName() string { return "p10_worship_policies" }

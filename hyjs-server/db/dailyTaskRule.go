package db

// DailyTaskRule 显式保存原服任务目标缺失时采用的本地日常规则。
type DailyTaskRule struct {
	ID       string `gorm:"primaryKey;size:64" json:"id"`
	Type     int32  `gorm:"not null" json:"type"`
	Total    int32  `gorm:"not null" json:"total"`
	Score    int32  `gorm:"not null" json:"score"`
	MinLevel int32  `gorm:"not null" json:"min_level"`
	Reward   string `gorm:"not null" json:"reward"`
	Enabled  bool   `gorm:"not null" json:"enabled"`
	Source   string `gorm:"not null" json:"source"`
}

func (DailyTaskRule) TableName() string { return "daily_task_rules" }

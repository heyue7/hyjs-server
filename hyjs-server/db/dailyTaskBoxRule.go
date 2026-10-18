package db

// DailyTaskBoxRule 恢复 APK 基础奖励，概率活动奖励另行定义。
type DailyTaskBoxRule struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	StartLevel int32  `gorm:"not null" json:"start_level"`
	EndLevel   int32  `gorm:"not null" json:"end_level"`
	Score      int32  `gorm:"not null" json:"score"`
	Reward     string `gorm:"not null" json:"reward"`
	Enabled    bool   `gorm:"not null" json:"enabled"`
	Source     string `gorm:"not null" json:"source"`
}

func (DailyTaskBoxRule) TableName() string { return "daily_task_box_rules" }

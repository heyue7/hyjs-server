package db

// P7RushStageRule 保存已识别原奖励与后续恢复的敌人；无敌人/条件证据的目录默认关闭。
type P7RushStageRule struct {
	ID               string  `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	Kind             int32   `gorm:"not null;index" json:"kind"`
	TaskID           string  `gorm:"type:varchar(64);not null" json:"taskId"`
	Floor            int32   `gorm:"not null" json:"floor"`
	Star             int32   `gorm:"not null" json:"star"`
	ConditionTaskID  string  `gorm:"type:varchar(64);not null" json:"conditionTaskId"`
	MinLevel         int32   `gorm:"not null" json:"minLevel"`
	Enabled          bool    `gorm:"not null" json:"enabled"`
	Scene            string  `gorm:"type:varbinary(64);not null" json:"scene"`
	MaxRound         int32   `gorm:"not null" json:"maxRound"`
	WinKey           int32   `gorm:"not null" json:"winKey"`
	WinValue         int32   `gorm:"not null" json:"winValue"`
	Enemies          []byte  `gorm:"type:longblob" json:"-"`
	FirstReward      string  `gorm:"type:text;not null" json:"firstReward"`
	Reward           string  `gorm:"type:text;not null" json:"reward"`
	RecommendedPower float64 `gorm:"not null" json:"recommendedPower"`
	Source           string  `gorm:"type:text;not null" json:"source"`
}

func (P7RushStageRule) TableName() string { return "p7_rush_stage_rules" }

package db

// P7ExpeditionStoryPolicy 明示未知剧情投放概率，既有任务继续使用冻结报价。
type P7ExpeditionStoryPolicy struct {
	ID           int64  `gorm:"primaryKey;autoIncrement:false"`
	Enabled      bool   `gorm:"not null;default:0"`
	LuckyPercent int32  `gorm:"not null"`
	Source       string `gorm:"type:text;not null"`
}

func (P7ExpeditionStoryPolicy) TableName() string { return "p7_expedition_story_policies" }

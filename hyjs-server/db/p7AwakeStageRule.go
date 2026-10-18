package db

// P7AwakeStageRule 保留具名来源；诊断导入与明确标记的本地实例目录各自冻结。
type P7AwakeStageRule struct {
	CopyID          string `gorm:"primaryKey;type:varchar(64)" json:"copyId"`
	ChapterID       string `gorm:"type:varchar(64);not null;index" json:"chapterId"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	ContentRestored bool   `gorm:"not null" json:"contentRestored"`
	FightPower      int64  `gorm:"not null" json:"fightPower"`
	Content         []byte `gorm:"type:longblob;not null" json:"-"`
	Source          string `gorm:"type:text;not null" json:"source"`
}

func (P7AwakeStageRule) TableName() string { return "p7_awake_stage_rules" }

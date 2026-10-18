package db

// P7ExpeditionEventPolicy 隔离未知原事件概率和数量，默认关闭并按任务冻结。
type P7ExpeditionEventPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false"`
	Enabled     bool   `gorm:"not null;default:0"`
	RiskPercent int32  `gorm:"not null"`
	HelpReward  string `gorm:"type:text;not null"`
	Source      string `gorm:"type:text;not null"`
}

func (P7ExpeditionEventPolicy) TableName() string { return "p7_expedition_event_policies" }

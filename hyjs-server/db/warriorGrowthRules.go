package db

// WarriorGrowthRule 明确保留 APK 费用与上限，SQL 只保存本地服务器补充的随机规则。
type WarriorGrowthRule struct {
	ID                 string `gorm:"column:id;primaryKey;size:32" json:"id"`
	Enabled            bool   `gorm:"not null" json:"enabled"`
	Distribution       string `gorm:"type:text;not null" json:"distribution"`
	CriticalMultiplier int64  `gorm:"not null" json:"criticalMultiplier"`
	ResetSeconds       int64  `gorm:"not null" json:"resetSeconds"`
	Source             string `gorm:"type:text;not null" json:"source"`
}

func (WarriorGrowthRule) TableName() string { return "warrior_growth_rules" }

// WarriorGrowthReceipt 保存每个角色和操作最近一次回执，防止原协议网络重试重复扣费。
type WarriorGrowthReceipt struct {
	PlayerID  int64  `gorm:"primaryKey" json:"playerId"`
	Guid      string `gorm:"primaryKey;size:128" json:"guid"`
	Kind      string `gorm:"primaryKey;size:32" json:"kind"`
	Timestamp int64  `gorm:"not null" json:"timestamp"`
	Request   []byte `gorm:"type:blob;not null" json:"request"`
	Response  []byte `gorm:"type:blob;not null" json:"response"`
}

func (WarriorGrowthReceipt) TableName() string { return "warrior_growth_receipts" }

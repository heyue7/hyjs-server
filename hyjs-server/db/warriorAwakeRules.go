package db

// WarriorAwakeRule 保存原服未恢复的补充费用与材料选择规则。
type WarriorAwakeRule struct {
	ID       string `gorm:"primaryKey;size:32" json:"id"`
	Enabled  bool   `gorm:"not null" json:"enabled"`
	CoinCost int64  `gorm:"not null" json:"coinCost"`
	Source   string `gorm:"type:text;not null" json:"source"`
}

func (WarriorAwakeRule) TableName() string { return "warrior_awake_rules" }

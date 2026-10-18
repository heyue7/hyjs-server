package db

// CookingRule 仅保存 APK 未提供的永久增幅策略，原材料、等级、金币和时效读取原表。
type CookingRule struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Gains   string `gorm:"type:text;not null" json:"gains"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (CookingRule) TableName() string { return "cooking_rules" }

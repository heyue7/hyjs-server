package db

// P9PetMarketRule 将缺失的运营目录与精灵币产出登记为具名可配置策略。
type P9PetMarketRule struct {
	ID      string `gorm:"primaryKey;size:32" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Input   []byte `gorm:"type:longblob;not null" json:"input"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P9PetMarketRule) TableName() string { return "p9_pet_market_rules" }

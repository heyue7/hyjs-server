package db

// TurntableRules 保存 APK 未给出的本地服规则，与原表价格和概率配置分开说明。
type TurntableRules struct {
	PrizePityRules string `gorm:"type:text" json:"prizePityRules"`
	PoolID         string `gorm:"column:pool_id;primaryKey;size:64" json:"poolId"`
	ScorePerDraw   int32  `gorm:"not null" json:"scorePerDraw"`
	TopPrize       string `gorm:"size:128;not null" json:"topPrize"`
	AdvancePrize   string `gorm:"size:128;not null" json:"advancePrize"`
	AdvancePlaces  int32  `gorm:"not null" json:"advancePlaces"`
	Description    string `gorm:"type:text;not null" json:"description"`
}

func (TurntableRules) TableName() string { return "turntable_rules" }

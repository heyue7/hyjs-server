package db

// P14PromotionPolicy 独立关闭的晋级赛资格/匹配点策略，原数值和具名首次发点边界由JSON/SHA冻结。
type P14PromotionPolicy struct {
	ID        int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled   bool   `gorm:"not null;default:false" json:"enabled"`
	Source    string `gorm:"type:text;not null" json:"source"`
	RulesJSON string `gorm:"type:longtext;not null" json:"rules_json"`
	RulesHash string `gorm:"type:char(64);not null" json:"rules_hash"`
}

func (P14PromotionPolicy) TableName() string { return "p14_promotion_policies" }

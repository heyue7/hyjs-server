package db

// P14PromotionFightPolicy explicitly publishes local scoring before any real match.
type P14PromotionFightPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null;default:''" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	RulesJSON   string `gorm:"type:longtext;not null" json:"rules_json"`
	RulesHash   string `gorm:"type:char(64);not null;default:''" json:"rules_hash"`
	PublishedAt int64  `gorm:"not null;default:0" json:"published_at"`
}

func (P14PromotionFightPolicy) TableName() string { return "p14_promotion_fight_policies" }

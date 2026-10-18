package db

// P14PromotionRankPolicy publishes the named local tie rule before phase7.
type P14PromotionRankPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null;default:''" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null;default:0" json:"published_at"`
}

func (P14PromotionRankPolicy) TableName() string { return "p14_promotion_rank_policies" }

package db

// GrowthRecruitPolicy publishes the existing OS fragment catalog without changing its frozen trades.
type GrowthRecruitPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	PublishedAt int64  `gorm:"not null;default:0" json:"published_at"`
	Version     string `gorm:"size:64;not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (GrowthRecruitPolicy) TableName() string { return "growth_recruit_policies" }

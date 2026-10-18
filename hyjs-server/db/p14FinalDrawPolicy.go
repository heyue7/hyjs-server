package db

// P14FinalDrawPolicy publishes the explicitly local draw rule before promotion.
type P14FinalDrawPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null;default:''" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null;default:0" json:"publishedAt"`
}

func (P14FinalDrawPolicy) TableName() string { return "p14_final_draw_policies" }

package db

// P14FinalFormationPolicy publishes the local first-round capture policy before its window.
type P14FinalFormationPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null;default:''" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null;default:0" json:"publishedAt"`
}

func (P14FinalFormationPolicy) TableName() string { return "p14_final_formation_policies" }

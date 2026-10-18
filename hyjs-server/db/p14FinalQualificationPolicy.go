package db

// P14FinalQualificationPolicy explicitly publishes the local sparse-roster rule before promotion.
type P14FinalQualificationPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null;default:''" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null;default:0" json:"published_at"`
}

func (P14FinalQualificationPolicy) TableName() string { return "p14_final_qualification_policies" }

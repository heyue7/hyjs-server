package db

// P14FinalSeriesPolicy binds real series scheduling to its frozen source, without presumed battle outcomes.
type P14FinalSeriesPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
}

func (P14FinalSeriesPolicy) TableName() string { return "p14_final_series_policies" }

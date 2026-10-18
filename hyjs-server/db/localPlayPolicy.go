package db

// LocalPlayPolicy controls the explicitly authorized local testing tools.
type LocalPlayPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Version string `gorm:"size:64;not null" json:"version"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (LocalPlayPolicy) TableName() string { return "local_play_policies" }

package db

// CopyBoxRule gates the original visible chest source independently of battle drops.
type CopyBoxRule struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (CopyBoxRule) TableName() string { return "copy_box_rules" }

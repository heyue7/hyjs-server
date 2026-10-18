package db

// P14GuessPolicy enables only a prepublished, explicitly local original-cost offer.
type P14GuessPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
}

func (P14GuessPolicy) TableName() string { return "p14_guess_policies" }

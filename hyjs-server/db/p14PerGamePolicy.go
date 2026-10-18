package db

// P14PerGamePolicy preserves independent real per-game sources without changing Game1 history.
type P14PerGamePolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
}

func (P14PerGamePolicy) TableName() string { return "p14_per_game_policies" }

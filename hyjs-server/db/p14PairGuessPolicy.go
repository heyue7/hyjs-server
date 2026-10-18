package db

// P14PairGuessPolicy preserves one actual prepublished pair source; it carries no fabricated result.
type P14PairGuessPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
}

func (P14PairGuessPolicy) TableName() string { return "p14_pair_guess_policies" }

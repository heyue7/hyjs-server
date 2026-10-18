package db

// P11LimitFightPolicy stores only current real mode20 publication, frozen input or accepted facts.
type P11LimitFightPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	Version     string `gorm:"type:varchar(64);not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"published__at"`
	StartAt     int64  `gorm:"not null" json:"start__at"`
	EndAt       int64  `gorm:"not null" json:"end__at"`
	MatchMillis int64  `gorm:"not null" json:"match__millis"`
}

func (P11LimitFightPolicy) TableName() string { return "p11_limit_fight_policies" }

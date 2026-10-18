package db

// P11GrabFightPolicy publishes the bounded original occupied-slot battle separately from empty occupancy.
type P11GrabFightPolicy struct {
	ID            int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled       bool   `gorm:"not null" json:"enabled"`
	Version       string `gorm:"type:varchar(64);not null" json:"version"`
	Source        string `gorm:"type:text;not null" json:"source"`
	PublishedAt   int64  `gorm:"not null" json:"published_at"`
	StartsAt      int64  `gorm:"not null" json:"starts_at"`
	EndsAt        int64  `gorm:"not null" json:"ends_at"`
	MaxRound      int32  `gorm:"not null" json:"max_round"`
	AttackerFirst bool   `gorm:"not null" json:"attacker_first"`
}

func (P11GrabFightPolicy) TableName() string { return "p11_grab_fight_policies" }

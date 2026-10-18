package db

// P11ArenaFightPolicy independently controls real rank-changing challenges.
// Enabling the existing friend-duel switches cannot enable this new consumer.
type P11ArenaFightPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Energy  int32  `gorm:"not null" json:"energy"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P11ArenaFightPolicy) TableName() string { return "p11_arena_fight_policies" }

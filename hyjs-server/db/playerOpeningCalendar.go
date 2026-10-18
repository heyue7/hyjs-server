package db

// PlayerOpeningCalendar freezes each character's first opening day in the game calendar.
// It does not modify profile creation timestamps or reset any claims.
type PlayerOpeningCalendar struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	StartAt   int64  `gorm:"not null" json:"start_at"`
	CreatedAt int64  `gorm:"not null;autoCreateTime:false" json:"created_at"`
	Source    string `gorm:"size:96;not null" json:"source"`
}

func (PlayerOpeningCalendar) TableName() string { return "player_opening_calendars" }

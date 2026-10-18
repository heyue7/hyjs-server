package db

// LocalPlayClockState stores one player's calendar offset without changing OS time.
type LocalPlayClockState struct {
	PlayerID   int64 `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	OffsetDays int64 `gorm:"not null" json:"offset_days"`
	Revision   int64 `gorm:"not null" json:"revision"`
	UpdatedAt  int64 `gorm:"not null" json:"updated_at"`
}

func (LocalPlayClockState) TableName() string { return "local_play_clock_states" }

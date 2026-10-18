package db

// LocalPlayClockOperation retains idempotent time changes across restart and retries.
type LocalPlayClockOperation struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	RequestID string `gorm:"primaryKey;size:128" json:"request_id"`
	Action    string `gorm:"size:32;not null" json:"action"`
	Response  []byte `gorm:"type:longblob;not null" json:"response"`
	CreatedAt int64  `gorm:"not null" json:"created_at"`
}

func (LocalPlayClockOperation) TableName() string { return "local_play_clock_operations" }

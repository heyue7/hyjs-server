package db

// GameLoginThrottle persists short account retry delays without storing credentials.
type GameLoginThrottle struct {
	AccountKey      string `gorm:"size:64;primaryKey"`
	Failures        int64  `gorm:"not null"`
	WindowStartedAt int64  `gorm:"not null"`
	RetryAt         int64  `gorm:"not null"`
	UpdatedAt       int64  `gorm:"not null;index"`
}

func (GameLoginThrottle) TableName() string { return "game_login_throttles" }

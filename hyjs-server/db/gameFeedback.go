package db

// GameFeedback stores a player-owned support conversation without game credentials.
type GameFeedback struct {
	ID        string `gorm:"primaryKey;type:varchar(36)"`
	PlayerID  int64  `gorm:"not null;index"`
	RequestID string `gorm:"type:varchar(36);not null;uniqueIndex:feedback_request,priority:2"`
	OwnerID   int64  `gorm:"not null;uniqueIndex:feedback_request,priority:1"`
	Category  string `gorm:"type:varchar(16);not null"`
	Subject   string `gorm:"type:varchar(320);not null"`
	State     string `gorm:"type:varchar(16);not null;index"`
	CreatedAt int64  `gorm:"not null;autoCreateTime:false"`
	UpdatedAt int64  `gorm:"not null;autoUpdateTime:false"`
}

func (GameFeedback) TableName() string { return "game_feedback" }

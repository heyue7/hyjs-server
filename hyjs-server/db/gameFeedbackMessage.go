package db

// GameFeedbackMessage freezes each submitted message; replies never replace earlier text.
type GameFeedbackMessage struct {
	ID         string `gorm:"primaryKey;type:varchar(36)"`
	FeedbackID string `gorm:"type:varchar(36);not null;index;uniqueIndex:feedback_message_request,priority:1"`
	RequestID  string `gorm:"type:varchar(36);not null;uniqueIndex:feedback_message_request,priority:2"`
	Sender     string `gorm:"type:varchar(16);not null"`
	ActorID    int64  `gorm:"not null"`
	Content    string `gorm:"type:text;not null"`
	CreatedAt  int64  `gorm:"not null;autoCreateTime:false"`
}

func (GameFeedbackMessage) TableName() string { return "game_feedback_messages" }

package db

// P10SocialChatState 原社交协议独立持久化域。
type P10SocialChatState struct {
	PlayerID   int64 `gorm:"primaryKey;autoIncrement:false"`
	LastSendAt int64 `gorm:"not null"`
}

func (P10SocialChatState) TableName() string { return "p10_social_chat_states" }

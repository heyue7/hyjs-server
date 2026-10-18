package db

// P10SocialChat 原社交协议独立持久化域。
type P10SocialChat struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	SenderID     int64  `gorm:"not null;index"`
	ReceiverID   int64  `gorm:"not null;index"`
	ChatType     int32  `gorm:"not null"`
	Content      string `gorm:"type:varchar(160);not null"`
	CreatedAt    int64  `gorm:"not null"`
	SenderInfo   []byte `gorm:"type:longblob;not null"`
	ReceiverName string `gorm:"type:varchar(128);not null"`
}

func (P10SocialChat) TableName() string { return "p10_social_chats" }

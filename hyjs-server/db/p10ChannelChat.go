package db

// P10ChannelChat 冻结原公共文字、发送人快照及服务器场景分线资格。
type P10ChannelChat struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	SenderID   int64  `gorm:"not null;index"`
	ChatType   int32  `gorm:"not null;index:idx_p10_channel_scope,priority:1"`
	Scene      string `gorm:"type:varchar(64);not null;index:idx_p10_channel_scope,priority:2"`
	Line       int32  `gorm:"not null;index:idx_p10_channel_scope,priority:3"`
	Content    string `gorm:"type:varchar(160);not null"`
	CreatedAt  int64  `gorm:"not null"`
	SenderInfo []byte `gorm:"type:longblob;not null"`
}

func (P10ChannelChat) TableName() string { return "p10_channel_chats" }

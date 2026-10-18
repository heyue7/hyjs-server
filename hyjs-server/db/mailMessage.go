package db

// MailMessage 保留原协议邮件字段，来源键防止离线重发；删除保留墓碑。
type MailMessage struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PlayerID  int64  `gorm:"not null;uniqueIndex:p1_mail_source,priority:1;uniqueIndex:p1_mail_delete,priority:1;index" json:"playerId"`
	SourceKey string `gorm:"type:varbinary(191);size:191;not null;uniqueIndex:p1_mail_source,priority:2" json:"sourceKey"`
	Mailbox   string `gorm:"size:32;not null" json:"mailbox"`
	Title     string `gorm:"size:191;not null" json:"title"`
	Content   string `gorm:"type:text;not null" json:"content"`
	TargetID  string `gorm:"size:191;not null" json:"targetId"`
	Type      int32  `gorm:"not null;uniqueIndex:p1_mail_delete,priority:2" json:"type"`
	CreatedAt int64  `gorm:"not null;uniqueIndex:p1_mail_delete,priority:3" json:"createdAt"`
	ExpiresAt int64  `gorm:"not null" json:"expiresAt"`
	ReadAt    int64  `gorm:"not null" json:"readAt"`
	DeletedAt int64  `gorm:"not null" json:"deletedAt"`
	RewardID  int64  `gorm:"not null" json:"rewardId"`
}

func (MailMessage) TableName() string { return "p1_mail_messages" }

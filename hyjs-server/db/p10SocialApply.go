package db

// P10SocialApply 原社交协议独立持久化域。
type P10SocialApply struct {
	SenderID   int64  `gorm:"primaryKey;autoIncrement:false"`
	ReceiverID int64  `gorm:"primaryKey;autoIncrement:false"`
	Word       string `gorm:"type:varchar(160);not null"`
	CreatedAt  int64  `gorm:"not null"`
}

func (P10SocialApply) TableName() string { return "p10_social_applies" }

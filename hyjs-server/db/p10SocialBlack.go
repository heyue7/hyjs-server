package db

// P10SocialBlack 原社交协议独立持久化域。
type P10SocialBlack struct {
	OwnerID   int64 `gorm:"primaryKey;autoIncrement:false"`
	TargetID  int64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt int64 `gorm:"not null"`
}

func (P10SocialBlack) TableName() string { return "p10_social_blacks" }

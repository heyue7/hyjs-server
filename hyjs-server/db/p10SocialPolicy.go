package db

// P10SocialPolicy 原社交协议独立持久化域。
type P10SocialPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false"`
	Enabled bool   `gorm:"not null"`
	Source  string `gorm:"type:text;not null"`
}

func (P10SocialPolicy) TableName() string { return "p10_social_policies" }

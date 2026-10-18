package db

// P10SocialRelation 原社交协议独立持久化域。
type P10SocialRelation struct {
	LowID     int64 `gorm:"primaryKey;autoIncrement:false"`
	HighID    int64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt int64 `gorm:"not null"`
}

func (P10SocialRelation) TableName() string { return "p10_social_relations" }

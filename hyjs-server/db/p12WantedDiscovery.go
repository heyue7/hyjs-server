package db

// P12WantedDiscovery 每个原副本成功事实仅抽一次，未命中也保存，重试不重新抽奖。
type P12WantedDiscovery struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	SourceKey      string `gorm:"primaryKey;type:varchar(64)" json:"source_key"`
	BusinessSource string `gorm:"type:varchar(192);not null" json:"business_source"`
	InstanceID     string `gorm:"type:varchar(64);not null" json:"instance_id"`
	Draw           int64  `gorm:"not null" json:"draw"`
	Odds           int64  `gorm:"not null" json:"odds"`
	Reason         string `gorm:"type:varchar(32);not null" json:"reason"`
	Reward         string `gorm:"type:text;not null" json:"reward"`
	CreatedAt      int64  `gorm:"not null" json:"created_at"`
	Source         string `gorm:"type:text;not null" json:"source"`
}

func (P12WantedDiscovery) TableName() string { return "p12_wanted_discoveries" }

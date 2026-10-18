package db

// P7SectionRewardPolicy 是原三档分段奖励的明确本地预发布契约。
type P7SectionRewardPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	Version     string `gorm:"size:64;not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
	StartAt     int64  `gorm:"not null" json:"startAt"`
	EndAt       int64  `gorm:"not null" json:"endAt"`
	SectionFrom int32  `gorm:"not null" json:"sectionFrom"`
	SectionTo   int32  `gorm:"not null" json:"sectionTo"`
	ThreeReward string `gorm:"type:text;not null" json:"threeReward"`
	SixReward   string `gorm:"type:text;not null" json:"sixReward"`
	NineReward  string `gorm:"type:text;not null" json:"nineReward"`
}

func (P7SectionRewardPolicy) TableName() string { return "p7_section_reward_policies" }

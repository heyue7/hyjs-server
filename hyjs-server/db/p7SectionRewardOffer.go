package db

// P7SectionRewardOffer 绑定真实轮次和首层付费冻结；领奖来源由本轮实际accepted胜利重核。
type P7SectionRewardOffer struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RunID          int64  `gorm:"primaryKey;autoIncrement:false" json:"runId"`
	FirstFightID   string `gorm:"size:64;not null" json:"firstFightId"`
	FirstStartHash string `gorm:"size:64;not null" json:"firstStartHash"`
	StartedAt      int64  `gorm:"not null" json:"startedAt"`
	Policy         []byte `gorm:"type:longblob;not null" json:"-"`
	StageRules     []byte `gorm:"type:longblob;not null" json:"-"`
	PolicyHash     string `gorm:"size:64;not null" json:"policyHash"`
	ClaimedAt      int64  `gorm:"not null" json:"claimedAt"`
	ClaimStamp     int64  `gorm:"not null" json:"claimStamp"`
	ClaimedStars   int32  `gorm:"not null" json:"claimedStars"`
	AcceptanceHash string `gorm:"size:64;not null" json:"acceptanceHash"`
}

func (P7SectionRewardOffer) TableName() string { return "p7_section_reward_offers" }

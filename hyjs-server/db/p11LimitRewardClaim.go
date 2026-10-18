package db

// P11LimitRewardClaim 真实累计积分档一次领取，回执由角色/活动/原奖励键唯一定位。
type P11LimitRewardClaim struct {
	PeriodID  string `gorm:"primaryKey;type:varchar(64)" json:"period_id"`
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	RewardKey int64  `gorm:"primaryKey;autoIncrement:false" json:"reward_key"`
	Score     int64  `gorm:"not null" json:"score"`
	Reward    string `gorm:"type:text;not null" json:"reward"`
	RulesHash string `gorm:"type:varchar(64);not null" json:"rules_hash"`
	Response  []byte `gorm:"type:longblob;not null" json:"response"`
	ClaimedAt int64  `gorm:"not null" json:"claimed_at"`
}

func (P11LimitRewardClaim) TableName() string { return "p11_limit_reward_claims" }

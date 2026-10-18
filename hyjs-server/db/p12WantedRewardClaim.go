package db

// P12WantedRewardClaim 绑定原每日目标和第一次实际领取，重登与重试返回同一结果。
type P12WantedRewardClaim struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day          string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	RewardID     int64  `gorm:"primaryKey;autoIncrement:false" json:"reward_id"`
	Reward       string `gorm:"type:text;not null" json:"reward"`
	Response     []byte `gorm:"type:longblob;not null" json:"response"`
	ResponseHash string `gorm:"type:varchar(64);not null" json:"response_hash"`
	CreatedAt    int64  `gorm:"not null" json:"created_at"`
	Source       string `gorm:"type:text;not null" json:"source"`
}

func (P12WantedRewardClaim) TableName() string { return "p12_wanted_reward_claims" }

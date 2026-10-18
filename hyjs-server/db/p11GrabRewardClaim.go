package db

// P11GrabRewardClaim 保存原奖杯争夺空位与实际占领分段，互斥占位不生成战斗赢家。
type P11GrabRewardClaim struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day        string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	Hours      int64  `gorm:"primaryKey;autoIncrement:false" json:"hours"`
	Millis     int64  `gorm:"not null" json:"millis"`
	Reward     string `gorm:"type:text;not null" json:"reward"`
	RewardHash string `gorm:"type:varchar(64);not null" json:"reward_hash"`
	Response   []byte `gorm:"type:longblob;not null" json:"response"`
	ClaimedAt  int64  `gorm:"not null" json:"claimed_at"`
}

func (P11GrabRewardClaim) TableName() string { return "p11_grab_reward_claims" }

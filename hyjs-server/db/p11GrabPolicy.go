package db

// P11GrabPolicy 保存原奖杯争夺空位与实际占领分段，互斥占位不生成战斗赢家。
type P11GrabPolicy struct {
	ID                int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled           bool   `gorm:"not null" json:"enabled"`
	TimeRewardEnabled bool   `gorm:"not null" json:"time_reward_enabled"`
	MaxTimes          int32  `gorm:"not null" json:"max_times"`
	BuyMax            int32  `gorm:"not null" json:"buy_max"`
	BuyPrice          int32  `gorm:"not null" json:"buy_price"`
	Energy            int32  `gorm:"not null" json:"energy"`
	MaxMillis         int64  `gorm:"not null" json:"max_millis"`
	RewardsJSON       string `gorm:"type:text;not null" json:"rewards_json"`
	RewardsHash       string `gorm:"type:varchar(64);not null" json:"rewards_hash"`
	Source            string `gorm:"type:text;not null" json:"source"`
}

func (P11GrabPolicy) TableName() string { return "p11_grab_policies" }

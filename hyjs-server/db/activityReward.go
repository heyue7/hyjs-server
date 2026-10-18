package db

// ActivityReward 将APK固定奖励保存在SQL，管理员可以热改尚未领取的档位。
type ActivityReward struct {
	ActivityID string `gorm:"primaryKey;size:64" json:"activityId"`
	RewardType string `gorm:"primaryKey;size:32" json:"rewardType"`
	Threshold  int32  `gorm:"primaryKey;autoIncrement:false" json:"threshold"`
	Reward     string `gorm:"size:512;not null" json:"reward"`
}

func (ActivityReward) TableName() string { return "activity_rewards" }

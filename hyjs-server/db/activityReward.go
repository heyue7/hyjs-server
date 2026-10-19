package db

// ActivityReward 将APK固定奖励保存在SQL，管理员可以热改尚未领取的档位。
type ActivityReward struct {
	ActivityID string `gorm:"primaryKey;size:64" json:"activityId"`
	RewardType string `gorm:"primaryKey;size:32" json:"rewardType"`
	Threshold  int32  `gorm:"primaryKey;autoIncrement:false" json:"threshold"`
	// RequiredCount 为全民福利的新人数门槛；Threshold 保留旧档位身份和领取键。
	RequiredCount int32  `gorm:"not null;default:0" json:"requiredCount"`
	Reward        string `gorm:"size:512;not null" json:"reward"`
}

func (ActivityReward) TableName() string { return "activity_rewards" }

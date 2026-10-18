package db

// ActivityRules 保存本地服明确选择的固定周期与原APK购买门槛。
type ActivityRules struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	MinVIP      int32  `gorm:"column:min_vip;not null" json:"minVip"`
	DiamondCost int32  `gorm:"not null" json:"diamondCost"`
	StartAt     int64  `gorm:"not null" json:"startAt"`
	EndAt       int64  `gorm:"not null" json:"endAt"`
	RewardEndAt int64  `gorm:"not null" json:"rewardEndAt"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (ActivityRules) TableName() string { return "activity_rules" }

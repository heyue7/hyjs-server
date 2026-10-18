package db

// PeriodProgress 冻结首次参与规则与累计/当天/单次口径，领奖引用同一持久化奖励ID。
type PeriodProgress struct {
	InstanceID string `gorm:"type:varbinary(64);primaryKey;size:64" json:"instanceId"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Snapshot   []byte `gorm:"type:longblob;not null" json:"-"`
	Value      int64  `gorm:"not null" json:"value"`
	LastDay    string `gorm:"size:10;not null" json:"lastDay"`
	RewardID   int64  `gorm:"not null" json:"rewardId"`
}

func (PeriodProgress) TableName() string { return "p1_period_progress" }

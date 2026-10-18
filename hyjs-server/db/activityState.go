package db

// ActivityState 的复合主键保证每个真实角色在同一活动仅有一份购买和领取状态。
type ActivityState struct {
	ActivityID string `gorm:"primaryKey;size:64" json:"activityId"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Bought     bool   `gorm:"not null" json:"bought"`
	Claims     []byte `gorm:"type:longblob" json:"claims"`
}

func (ActivityState) TableName() string { return "activity_states" }

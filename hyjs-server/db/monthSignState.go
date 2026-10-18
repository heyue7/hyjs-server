package db

// MonthSignState 保留月累计和当天奖励快照，角色行锁串行化签到领取。
type MonthSignState struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Month         string `gorm:"primaryKey;size:7" json:"month"`
	Days          int32  `gorm:"not null" json:"days"`
	LastDay       string `gorm:"size:10;not null" json:"last_day"`
	Doubled       bool   `gorm:"not null" json:"doubled"`
	LastReward    string `gorm:"not null" json:"last_reward"`
	LastDoubleVIP int32  `gorm:"column:last_double_vip;not null" json:"last_double_vip"`
}

func (MonthSignState) TableName() string { return "month_sign_states" }

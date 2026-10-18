package db

// P4WelfareState 以玩家/独立实例保存领取、冷却和冻结奖历，重登不重新初始化奖励。
type P4WelfareState struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RuleID   string `gorm:"primaryKey;type:varbinary(64)" json:"ruleId"`
	Snapshot []byte `gorm:"type:longblob;not null" json:"snapshot"`
	Payload  []byte `gorm:"type:longblob;not null" json:"payload"`
}

func (P4WelfareState) TableName() string { return "p4_welfare_states" }

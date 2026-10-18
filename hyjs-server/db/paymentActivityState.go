package db

// PaymentActivityState 首次观察即冻结门槛、窗口和奖励；后续配置不改变已见实例。
type PaymentActivityState struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	ActivityID string `gorm:"primaryKey;size:128" json:"activity_id"`
	Snapshot   string `gorm:"type:longtext;not null" json:"snapshot"`
	Claimed    bool   `gorm:"not null" json:"claimed"`
}

func (PaymentActivityState) TableName() string { return "p3_payment_activity_states" }

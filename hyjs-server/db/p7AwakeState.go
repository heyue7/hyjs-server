package db

// P7AwakeState 角色日预算，查询不扣次数、不创建探索实例。
type P7AwakeState struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Day      string `gorm:"type:char(10);not null" json:"day"`
	Used     int32  `gorm:"not null" json:"used"`
}

func (P7AwakeState) TableName() string { return "p7_awake_states" }

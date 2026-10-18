package db

// P7RushProgress 保存实际权威结算达成的任务，精英开放条件引用任务编号而非客户端星数。
type P7RushProgress struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Kind       int32  `gorm:"primaryKey" json:"kind"`
	TaskID     string `gorm:"primaryKey;type:varchar(64)" json:"taskId"`
	Star       int32  `gorm:"not null" json:"star"`
	FirstFight string `gorm:"type:varbinary(64);not null" json:"firstFight"`
}

func (P7RushProgress) TableName() string { return "p7_rush_progress" }

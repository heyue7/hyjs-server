package db

// OpenServerActivityState 分离真实达成快照与已领取数量，宽限期不接受新进度。
type OpenServerActivityState struct {
	PlayerID int64 `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	TaskID   int32 `gorm:"primaryKey;autoIncrement:false" json:"task_id"`
	Achieved bool  `gorm:"not null" json:"achieved"`
	Progress int32 `gorm:"not null" json:"progress"`
	Claims   int32 `gorm:"not null" json:"claims"`
}

func (OpenServerActivityState) TableName() string { return "open_server_activity_states" }

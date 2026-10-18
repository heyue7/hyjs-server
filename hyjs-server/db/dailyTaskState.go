package db

// DailyTaskState 将当天条件、计数、奖励和领取记录冻结为完整快照。
type DailyTaskState struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day      string `gorm:"primaryKey;size:10" json:"day"`
	Snapshot string `gorm:"type:longtext;not null" json:"snapshot"`
}

func (DailyTaskState) TableName() string { return "daily_task_states" }

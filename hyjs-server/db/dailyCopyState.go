package db

// DailyCopyState 按原表类型共享三个难度的每日次数，开始时计次并冻结当日上限。
type DailyCopyState struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day      string `gorm:"primaryKey;size:10" json:"day"`
	Type     string `gorm:"primaryKey;size:64" json:"type"`
	Used     int32  `gorm:"not null" json:"used"`
	Limit    int32  `gorm:"not null" json:"limit"`
}

func (DailyCopyState) TableName() string { return "daily_copy_states" }

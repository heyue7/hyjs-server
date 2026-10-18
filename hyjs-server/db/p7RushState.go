package db

// P7RushState 保存日次数、当前轮次和真实星数，查询只恢复日期，不制造挑战或通关。
type P7RushState struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Day           string `gorm:"type:char(10);not null" json:"day"`
	RunID         int64  `gorm:"not null" json:"runId"`
	Floor         int32  `gorm:"not null" json:"floor"`
	TotalStars    int32  `gorm:"not null" json:"totalStars"`
	RecordStars   int32  `gorm:"not null" json:"recordStars"`
	HighFloor     int32  `gorm:"not null" json:"highFloor"`
	FreeResetUsed int32  `gorm:"not null" json:"freeResetUsed"`
	PaidResetUsed int32  `gorm:"not null" json:"paidResetUsed"`
	EliteUsed     int32  `gorm:"not null" json:"eliteUsed"`
	EliteBought   int32  `gorm:"not null" json:"eliteBought"`
	ActiveFight   string `gorm:"type:varbinary(64);not null" json:"activeFight"`
}

func (P7RushState) TableName() string { return "p7_rush_states" }

package db

// P7ReRushRun 保存本轮一次性商人状态，重置通过新runID自然隔离。
type P7ReRushRun struct {
	PlayerID     int64 `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RunID        int64 `gorm:"primaryKey;autoIncrement:false" json:"runId"`
	TraderBought bool  `gorm:"not null" json:"traderBought"`
}

func (P7ReRushRun) TableName() string { return "p7_re_rush_runs" }

package db

// P7ReRushSweep 冻结历史胜利或阵容战力证明支撑的扫荡楼层。
type P7ReRushSweep struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RunID       int64  `gorm:"primaryKey;autoIncrement:false" json:"runId"`
	Floor       int32  `gorm:"primaryKey;autoIncrement:false" json:"floor"`
	Star        int32  `gorm:"not null" json:"star"`
	SourceFight string `gorm:"size:64;not null" json:"sourceFight"`
	SourceHash  string `gorm:"size:64;not null" json:"sourceHash"`
}

func (P7ReRushSweep) TableName() string { return "p7_re_rush_sweeps" }

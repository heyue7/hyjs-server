package db

// P7RushFight 冻结任务/奖励/困难度和原始开始帧，诊断实例永远不能发奖或占用次数。
type P7RushFight struct {
	ID         string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	PlayerID   int64  `gorm:"not null;index" json:"playerId"`
	Kind       int32  `gorm:"not null" json:"kind"`
	TaskID     string `gorm:"type:varchar(64);not null" json:"taskId"`
	Status     string `gorm:"type:varbinary(16);not null" json:"status"`
	Diagnostic bool   `gorm:"not null" json:"diagnostic"`
	Day        string `gorm:"type:char(10);not null" json:"day"`
	RunID      int64  `gorm:"not null" json:"runId"`
	Rule       []byte `gorm:"type:longblob;not null" json:"-"`
	RuleHash   string `gorm:"type:char(64);not null" json:"ruleHash"`
	Start      []byte `gorm:"type:longblob;not null" json:"-"`
	StartHash  string `gorm:"type:char(64);not null" json:"startHash"`
	Result     []byte `gorm:"type:longblob" json:"-"`
	ReplayID   string `gorm:"type:char(64);not null" json:"replayId"`
	EndRequest []byte `gorm:"type:longblob" json:"-"`
}

func (P7RushFight) TableName() string { return "p7_rush_fights" }

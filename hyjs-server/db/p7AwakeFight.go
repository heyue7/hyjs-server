package db

// P7AwakeFight 冻结原typed16开战与完整End回执；P6Record保存同一完整计算输入。
type P7AwakeFight struct {
	ID           string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	PlayerID     int64  `gorm:"not null;index" json:"playerId"`
	RunID        string `gorm:"type:varbinary(64);not null;index" json:"runId"`
	MonsterGUID  string `gorm:"type:varbinary(64);not null" json:"monsterGuid"`
	Status       string `gorm:"type:varchar(16);not null" json:"status"`
	ReplayID     string `gorm:"type:varchar(64);not null" json:"replayId"`
	ScopeProof   string `gorm:"type:char(64);not null" json:"scopeProof"`
	Start        []byte `gorm:"type:longblob;not null" json:"-"`
	StartHash    string `gorm:"type:char(64);not null" json:"startHash"`
	InputHash    string `gorm:"type:char(64);not null" json:"inputHash"`
	Terminal     []byte `gorm:"type:longblob" json:"-"`
	TerminalHash string `gorm:"type:char(64);not null" json:"terminalHash"`
	EndRequest   []byte `gorm:"type:longblob" json:"-"`
	EndHash      string `gorm:"type:char(64);not null" json:"endHash"`
	Reply        []byte `gorm:"type:longblob" json:"-"`
	ReplyHash    string `gorm:"type:char(64);not null" json:"replyHash"`
	CreatedAt    int64  `gorm:"not null" json:"createdAt"`
	AcceptedAt   int64  `gorm:"not null" json:"acceptedAt"`
}

func (P7AwakeFight) TableName() string { return "p7_awake_fights" }

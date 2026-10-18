package db

// P7AwakeExploration 保存实际探索进度和一次结算回执；节点及抽奖结果继续由 Run 冻结。
type P7AwakeExploration struct {
	RunID       string `gorm:"primaryKey;type:varbinary(64)" json:"runId"`
	PlayerID    int64  `gorm:"not null;index:idx_awake_player_status;index:idx_awake_player_settle_stamp" json:"playerId"`
	Status      string `gorm:"type:varchar(16);not null;index:idx_awake_player_status" json:"status"`
	State       []byte `gorm:"type:longblob;not null" json:"-"`
	StateHash   string `gorm:"type:char(64);not null" json:"stateHash"`
	EnterStamp  int64  `gorm:"not null" json:"enterStamp"`
	SettleStamp int64  `gorm:"not null;index:idx_awake_player_settle_stamp" json:"settleStamp"`
	SettledAt   int64  `gorm:"not null" json:"settledAt"`
	Receipt     []byte `gorm:"type:longblob" json:"-"`
	ReceiptHash string `gorm:"type:char(64);not null" json:"receiptHash"`
}

func (P7AwakeExploration) TableName() string { return "p7_awake_explorations" }

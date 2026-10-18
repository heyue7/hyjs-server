package db

// P7AwakeRun 冻结节点/原表规则/角色帧；诊断实例无资源，真实探索由独立账本结算。
type P7AwakeRun struct {
	ID         string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	PlayerID   int64  `gorm:"not null;index" json:"playerId"`
	CopyID     string `gorm:"type:varchar(64);not null" json:"copyId"`
	Diagnostic bool   `gorm:"not null" json:"diagnostic"`
	StartAt    int64  `gorm:"not null" json:"startAt"`
	EndAt      int64  `gorm:"not null" json:"endAt"`
	Freeze     []byte `gorm:"type:longblob;not null" json:"-"`
	FreezeHash string `gorm:"type:char(64);not null" json:"freezeHash"`
	Actor      []byte `gorm:"type:longblob;not null" json:"-"`
	EnterFrame []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P7AwakeRun) TableName() string { return "p7_awake_runs" }

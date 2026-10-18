package db

// PeriodSettlement 截止排名只冻结一次，快照与离线奖励投递一起提交。
type PeriodSettlement struct {
	InstanceID string `gorm:"type:varbinary(64);primaryKey;size:64" json:"instanceId"`
	SettledAt  int64  `gorm:"not null" json:"settledAt"`
	Snapshot   []byte `gorm:"type:longblob;not null" json:"-"`
}

func (PeriodSettlement) TableName() string { return "p1_period_settlements" }

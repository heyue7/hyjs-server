package db

// P8GatherAttempt 将采集计时和已经抽定的实际奖励持久化，断线不重新抽取。
type P8GatherAttempt struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Token      string `gorm:"size:64;not null" json:"token"`
	Scene      string `gorm:"size:64;not null" json:"scene"`
	PosID      string `gorm:"size:64;not null" json:"posId"`
	Generation int64  `gorm:"not null" json:"generation"`
	StartedAt  int64  `gorm:"not null" json:"startedAt"`
	ReadyAt    int64  `gorm:"not null" json:"readyAt"`
	Active     bool   `gorm:"not null" json:"active"`
	Snapshot   string `gorm:"type:longtext;not null" json:"-"`
	Rewards    string `gorm:"type:longtext;not null" json:"-"`
}

func (P8GatherAttempt) TableName() string { return "p8_gather_attempts" }

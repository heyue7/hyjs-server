package db

// P8GatherAttempt 将采集计时和已经抽定的实际奖励持久化，断线不重新抽取。
type P8GatherAttempt struct {
	PlayerID           int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Token              string `gorm:"size:64;not null" json:"token"`
	Scene              string `gorm:"size:64;not null;index:idx_p8_gather_due,priority:1;index:idx_p8_gather_point,priority:1" json:"scene"`
	PosID              string `gorm:"size:64;not null;index:idx_p8_gather_point,priority:2" json:"posId"`
	Generation         int64  `gorm:"not null" json:"generation"`
	StartedAt          int64  `gorm:"not null" json:"startedAt"`
	ReadyAt            int64  `gorm:"not null;index:idx_p8_gather_due,priority:3;index:idx_p8_gather_point,priority:4" json:"readyAt"`
	Active             bool   `gorm:"not null;index:idx_p8_gather_due,priority:2;index:idx_p8_gather_point,priority:3" json:"active"`
	ReservationVersion int32  `gorm:"not null;default:0" json:"-"`
	ReservationLine    int32  `gorm:"not null;default:0" json:"-"`
	Snapshot           string `gorm:"type:longtext;not null" json:"-"`
	Rewards            string `gorm:"type:longtext;not null" json:"-"`
}

func (P8GatherAttempt) TableName() string { return "p8_gather_attempts" }

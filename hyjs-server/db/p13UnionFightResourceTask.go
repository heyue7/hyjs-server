package db

// P13UnionFightResourceTask 记录原采集开始/结束与一次结果，重启/并发tick不重复取得资源。
type P13UnionFightResourceTask struct {
	ID         string `gorm:"primaryKey;type:char(64);index:idx_resource_task_pending,priority:3" json:"id"`
	MatchID    string `gorm:"type:char(64);not null;index" json:"matchId"`
	PlayerID   int64  `gorm:"not null;index" json:"playerId"`
	NodeGUID   int64  `gorm:"not null" json:"nodeGuid"`
	StartedAt  int64  `gorm:"not null" json:"startedAt"`
	DueAt      int64  `gorm:"not null;index;index:idx_resource_task_pending,priority:2" json:"dueAt"`
	FinishedAt int64  `gorm:"not null;default:0" json:"finishedAt"`
	Status     int64  `gorm:"not null;default:0;index:idx_resource_task_pending,priority:1" json:"status"` // 0=采集中，1=实际完成，2=撤销且未取得资源。
	Granted    int64  `gorm:"not null;default:0" json:"granted"`
	Result     []byte `gorm:"type:blob" json:"-"`
	ResultHash string `gorm:"type:char(64);not null;default:''" json:"resultHash"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightResourceTask) TableName() string { return "p13_union_fight_resource_tasks" }

package db

// P13UnionFightTransferTask 固定原传送点/目标/时刻，一次完成后只返回历史帧。
type P13UnionFightTransferTask struct {
	ID         string `gorm:"primaryKey;type:char(64);index:idx_uf_transfer_pending,priority:3" json:"id"`
	MatchID    string `gorm:"type:char(64);not null;index" json:"matchId"`
	PlayerID   int64  `gorm:"not null;index" json:"playerId"`
	Ordinal    int64  `gorm:"not null" json:"ordinal"`
	TransferID string `gorm:"type:varchar(64);not null" json:"transferId"`
	Target     []byte `gorm:"type:blob;not null" json:"-"`
	StartedAt  int64  `gorm:"not null" json:"startedAt"`
	DueAt      int64  `gorm:"not null;index:idx_uf_transfer_pending,priority:2" json:"dueAt"`
	FinishedAt int64  `gorm:"not null;default:0" json:"finishedAt"`
	Status     int64  `gorm:"not null;default:0;index:idx_uf_transfer_pending,priority:1" json:"status"` // 0=计时中，1=已完成，2=资格撤销/退场取消。
	SourceHash string `gorm:"type:char(64);not null" json:"sourceHash"`
	Result     []byte `gorm:"type:blob" json:"-"`
	ResultHash string `gorm:"type:char(64);not null;default:''" json:"resultHash"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightTransferTask) TableName() string { return "p13_union_fight_transfer_tasks" }

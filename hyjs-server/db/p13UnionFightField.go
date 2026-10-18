package db

// P13UnionFightField 固定真实MatchID的完整原建筑与地图快照，只有场景状态，无默认胜负。
type P13UnionFightField struct {
	MatchID      string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	Snapshot     []byte `gorm:"type:longblob;not null" json:"-"`
	SnapshotHash string `gorm:"type:char(64);not null" json:"snapshotHash"`
	CreatedAt    int64  `gorm:"not null" json:"createdAt"`
	Source       string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightField) TableName() string { return "p13_union_fight_fields" }

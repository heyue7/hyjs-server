package db

// P11ArenaMember 名次来自真实入列或后续权威对战，查询不写胜利和奖励事实。
type P11ArenaMember struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false"`
	Rank         int64  `gorm:"column:position;not null;uniqueIndex:idx_p11_arena_position"`
	Active       bool   `gorm:"not null"`
	Info         []byte `gorm:"type:longblob;not null"`
	Battle       []byte `gorm:"type:longblob;not null"`
	SnapshotHash string `gorm:"type:varchar(64);not null"`
	CreatedAt    int64  `gorm:"not null"`
	UpdatedAt    int64  `gorm:"autoUpdateTime:false;not null"`
	BestWonRank  int64  `gorm:"not null"`
}

func (P11ArenaMember) TableName() string { return "p11_arena_members" }

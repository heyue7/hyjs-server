package db

// KingRecruitActivitySnapshot freezes the original fixed rules for a scheduled
// costType3 pool independently of later global catalogue edits.
type KingRecruitActivitySnapshot struct {
	PoolID  string `gorm:"primaryKey;size:64" json:"poolId"`
	Payload []byte `gorm:"type:longblob;not null" json:"-"`
	Frozen  bool   `gorm:"not null" json:"frozen"`
}

func (KingRecruitActivitySnapshot) TableName() string { return "king_recruit_activity_snapshots" }

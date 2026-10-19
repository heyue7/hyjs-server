package db

// KingRecruitSelection preserves a role's explicit world independently of shared draw counters.
type KingRecruitSelection struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	PoolID   string `gorm:"size:128;not null" json:"poolId"`
}

func (KingRecruitSelection) TableName() string { return "king_recruit_selections" }

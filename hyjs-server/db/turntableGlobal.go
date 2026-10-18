package db

// TurntableGlobal 的单行锁串行化共享钻石池、有限库存及最终排名发奖。
type TurntableGlobal struct {
	PoolID                string `gorm:"column:pool_id;primaryKey;size:64" json:"poolId"`
	DiamondPool           int64  `gorm:"not null" json:"diamondPool"`
	ContributionRemainder int64  `gorm:"not null" json:"contributionRemainder"`
	UsedStock             []byte `gorm:"type:longblob" json:"usedStock"`
	Settled               bool   `gorm:"not null" json:"settled"`
	FinalRanks            []byte `gorm:"type:longblob" json:"finalRanks"`
}

func (TurntableGlobal) TableName() string { return "turntable_globals" }

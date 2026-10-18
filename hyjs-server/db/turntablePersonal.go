package db

// TurntablePersonal owns a player's inventory and jackpot independently from legacy shared pools.
type TurntablePersonal struct {
	PoolID                string `gorm:"primaryKey;size:64" json:"poolId"`
	PlayerID              int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	DiamondPool           int64  `gorm:"not null" json:"diamondPool"`
	ContributionRemainder int64  `gorm:"not null" json:"contributionRemainder"`
	UsedStock             []byte `gorm:"type:longblob" json:"usedStock"`
	Settled               bool   `gorm:"not null" json:"settled"`
	LegacyScore           int32  `gorm:"not null" json:"-"`
	CreatedAt             int64  `gorm:"not null" json:"createdAt"`
}

func (TurntablePersonal) TableName() string { return "turntable_personals" }

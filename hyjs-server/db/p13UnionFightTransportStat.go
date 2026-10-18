package db

// P13UnionFightTransportStat counts actual deposited ore independently of spendable camp/token balances.
type P13UnionFightTransportStat struct {
	MatchID   string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	GuildID   string `gorm:"type:varchar(64);not null" json:"guildId"`
	Ore       int64  `gorm:"not null" json:"ore"`
	Deposits  int64  `gorm:"not null" json:"deposits"`
	UpdatedAt int64  `gorm:"not null;autoUpdateTime:false" json:"updatedAt"`
}

func (P13UnionFightTransportStat) TableName() string { return "p13_union_fight_transport_stats" }

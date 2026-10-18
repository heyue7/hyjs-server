package db

// P13UnionFightGuildMissionFact records an actual newly opened field factory and its frozen task prestige.
type P13UnionFightGuildMissionFact struct {
	MatchID     string `gorm:"primaryKey;type:char(64)" json:"match_id"`
	GuildID     string `gorm:"primaryKey;type:varchar(64)" json:"guild_id"`
	FactoryGUID int64  `gorm:"not null" json:"factory_guid"`
	PlayerID    int64  `gorm:"not null" json:"player_id"`
	Prestige    int32  `gorm:"not null" json:"prestige"`
	OfferHash   string `gorm:"type:char(64);not null" json:"offer_hash"`
	OccurredAt  int64  `gorm:"not null" json:"occurred_at"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightGuildMissionFact) TableName() string { return "p13_union_fight_guild_mission_facts" }

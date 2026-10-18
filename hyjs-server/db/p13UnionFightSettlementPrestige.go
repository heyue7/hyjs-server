package db

// P13UnionFightSettlementPrestige 保存具名运营零战斗终局及其真实发奖来源。
type P13UnionFightSettlementPrestige struct {
	WeekStart string `gorm:"primaryKey;type:varchar(10)" json:"week_start"`
	GuildID   string `gorm:"primaryKey;type:varchar(64)" json:"guild_id"`
	Score     int64  `gorm:"not null;default:0" json:"score"`
}

func (P13UnionFightSettlementPrestige) TableName() string {
	return "p13_union_fight_settlement_prestiges"
}

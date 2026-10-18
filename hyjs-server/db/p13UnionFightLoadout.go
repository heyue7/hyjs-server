package db

// P13UnionFightLoadout 保存真实匹配角色的每轮道具种类；配置阶段不提前消费背包库存。
type P13UnionFightLoadout struct {
	MatchID    string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	ItemIDs    string `gorm:"type:varchar(512);not null" json:"itemIds"`
	PreparedAt int64  `gorm:"not null" json:"preparedAt"`
}

func (P13UnionFightLoadout) TableName() string { return "p13_union_fight_loadouts" }

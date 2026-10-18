package db

// P13UnionFightShopVisit 将原购买后空npcId刷新绑定到真实授权的本阵营商店。
type P13UnionFightShopVisit struct {
	MatchID   string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	NPCID     string `gorm:"type:varchar(64);not null" json:"npcId"`
	VisitedAt int64  `gorm:"not null" json:"visitedAt"`
}

func (P13UnionFightShopVisit) TableName() string { return "p13_union_fight_shop_visits" }

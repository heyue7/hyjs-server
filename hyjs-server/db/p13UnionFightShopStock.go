package db

// P13UnionFightShopStock 保存原每名成员逐次恢复的购买余量；CD来源于明确下发间隔。
type P13UnionFightShopStock struct {
	MatchID       string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	ShopItemID    string `gorm:"primaryKey;type:varchar(64)" json:"shopItemId"`
	LeftCount     int64  `gorm:"not null" json:"leftCount"`
	LastRefreshAt int64  `gorm:"not null" json:"lastRefreshAt"`
}

func (P13UnionFightShopStock) TableName() string { return "p13_union_fight_shop_stocks" }

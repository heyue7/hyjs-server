package db

// P13UnionFightShopOffer 冻结本场原UF物品身份与明确报价，后续发布不改当前实例。
type P13UnionFightShopOffer struct {
	MatchID    string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	ShopItemID string `gorm:"primaryKey;type:varchar(64)" json:"shopItemId"`
	Frame      []byte `gorm:"type:blob;not null" json:"-"`
	SourceHash string `gorm:"type:char(64);not null" json:"sourceHash"`
	FrameHash  string `gorm:"type:char(64);not null" json:"frameHash"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightShopOffer) TableName() string { return "p13_union_fight_shop_offers" }

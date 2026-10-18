package db

// P13UnionFightShopSource 保存运营明确的具名本地报价；原APK仅有下发协议，无报价表种子。
type P13UnionFightShopSource struct {
	Day           string `gorm:"primaryKey;type:char(10)" json:"day"`
	ShopItemID    string `gorm:"primaryKey;type:varchar(64)" json:"shopItemId"`
	Enabled       bool   `gorm:"not null;default:false" json:"enabled"`
	ItemID        string `gorm:"type:varchar(64);not null" json:"itemId"`
	Price         int64  `gorm:"not null" json:"price"`
	Quantity      int64  `gorm:"not null" json:"quantity"`
	BuyCount      int64  `gorm:"not null" json:"buyCount"`
	BuyCondition  int64  `gorm:"not null" json:"buyCondition"` // 0=全部成员，1=副会长/会长，2=会长。
	Priority      int64  `gorm:"not null" json:"priority"`
	RefreshMillis int64  `gorm:"not null" json:"refreshMillis"`
	PublishedAt   int64  `gorm:"not null" json:"publishedAt"`
	Source        string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightShopSource) TableName() string { return "p13_union_fight_shop_sources" }

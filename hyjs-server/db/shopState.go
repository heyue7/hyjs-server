package db

// ShopState 按真实北京时间日期保存每位玩家每种商品的已购次数。
type ShopState struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	ProductID string `gorm:"primaryKey;size:64" json:"productId"`
	Day       string `gorm:"primaryKey;size:10" json:"day"`
	Bought    int32  `gorm:"not null" json:"bought"`
}

func (ShopState) TableName() string { return "shop_states" }

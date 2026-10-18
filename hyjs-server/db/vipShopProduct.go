package db

// VIPShopProduct 显式本地会员商品；每角色每日快照复用PaymentActivityState。
type VIPShopProduct struct {
	ID      string `gorm:"primaryKey;size:64" json:"id"`
	MinVIP  int64  `gorm:"not null" json:"min_vip"`
	Cost    int64  `gorm:"not null" json:"cost"`
	Reward  string `gorm:"type:text;not null" json:"reward"`
	Source  string `gorm:"type:text;not null" json:"source"`
	Enabled bool   `gorm:"not null" json:"enabled"`
}

func (VIPShopProduct) TableName() string { return "p3_vip_shop_products" }

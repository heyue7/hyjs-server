package db

// VIPShopProduct 保存实时会员商品；旧PaymentActivityState仅保留旧屏幕报价见证。
type VIPShopProduct struct {
	ID           string `gorm:"primaryKey;size:64" json:"id"`
	MinVIP       int64  `gorm:"not null" json:"min_vip"`
	CostResource string `gorm:"size:64;not null;default:''" json:"cost_resource"`
	TotalLimit   int64  `gorm:"not null;default:0" json:"totalLimit"` // 0 keeps daily purchase; positive caps lifetime claims of this stable product.
	Cost         int64  `gorm:"not null" json:"cost"`
	Reward       string `gorm:"type:text;not null" json:"reward"`
	Source       string `gorm:"type:text;not null" json:"source"`
	Enabled      bool   `gorm:"not null" json:"enabled"`
}

func (VIPShopProduct) TableName() string { return "p3_vip_shop_products" }

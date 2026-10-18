package db

// ShopProduct 保存本地服目录与限购；固定客户端价格须与 APK 表一致。
type ShopProduct struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	Source     string `gorm:"size:255;not null" json:"source"`
	Enabled    bool   `gorm:"not null" json:"enabled"`
	ResourceID string `gorm:"size:64;not null" json:"resourceId"`
	Amount     int32  `gorm:"not null" json:"amount"`
	Cost       string `gorm:"size:128;not null" json:"cost"`
	DailyLimit int32  `gorm:"not null" json:"dailyLimit"`
	MinLevel   int32  `gorm:"not null" json:"minLevel"`
	MinVIP     int32  `gorm:"column:min_vip;not null" json:"minVip"`
	Priority   int32  `gorm:"not null" json:"priority"`
}

func (ShopProduct) TableName() string { return "shop_products" }

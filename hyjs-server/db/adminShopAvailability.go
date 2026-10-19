package db

// AdminShopAvailability controls business hours independently from immutable catalogues and purchase history.
type AdminShopAvailability struct {
	ShopKey    string `gorm:"primaryKey;size:32" json:"shopKey"`
	Unlimited  bool   `gorm:"not null" json:"unlimited"`
	StartAt    int64  `gorm:"not null" json:"startAt"`
	EndAt      int64  `gorm:"not null" json:"endAt"`
	ClaimEndAt int64  `gorm:"not null" json:"claimEndAt"`
	Revision   int64  `gorm:"not null" json:"-"`
}

func (AdminShopAvailability) TableName() string { return "admin_shop_availability" }

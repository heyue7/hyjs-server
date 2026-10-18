package db

// PaymentProduct 保存渠道原表及明示本地测试商品；订单另外冻结此快照。
type PaymentProduct struct {
	ID          string `gorm:"primaryKey;size:128" json:"id"`
	Channels    string `gorm:"type:text;not null" json:"channels"`
	Shop        string `gorm:"size:32;not null" json:"shop"`
	Kind        string `gorm:"size:32;not null" json:"kind"`
	Cents       int64  `gorm:"not null" json:"cents"`
	Currency    string `gorm:"size:8;not null" json:"currency"`
	Charge      int64  `gorm:"not null" json:"charge"`
	Free        int64  `gorm:"not null" json:"free"`
	Reward      string `gorm:"type:text;not null" json:"reward"`
	Name        string `gorm:"type:text;not null" json:"name"`
	Description string `gorm:"type:text;not null" json:"description"`
	Source      string `gorm:"type:text;not null" json:"source"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	FirstDouble bool   `gorm:"not null" json:"first_double"`
}

func (PaymentProduct) TableName() string { return "p3_payment_products" }

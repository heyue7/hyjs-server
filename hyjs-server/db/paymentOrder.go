package db

// PaymentOrder 用角色与请求时间戳唯一键生成订单；只可信确认事务能改为confirmed。
type PaymentOrder struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	PlayerID    int64  `gorm:"uniqueIndex:p3_order_request;not null" json:"player_id"`
	RequestKey  int64  `gorm:"uniqueIndex:p3_order_request;not null" json:"request_key"`
	RequestHash string `gorm:"size:64;not null" json:"request_hash"`
	ProductID   string `gorm:"size:128;not null" json:"product_id"`
	Snapshot    string `gorm:"type:longtext;not null" json:"snapshot"`
	Status      string `gorm:"size:16;not null" json:"status"`
	CreatedAt   int64  `gorm:"not null;autoCreateTime:false" json:"created_at"`
	ExpiresAt   int64  `gorm:"not null" json:"expires_at"`
	ConfirmedAt int64  `gorm:"not null" json:"confirmed_at"`
	Cents       int64  `gorm:"not null" json:"cents"`
	Credited    int64  `gorm:"not null" json:"credited"`
	Source      string `gorm:"size:32;not null" json:"source"`
}

func (PaymentOrder) TableName() string { return "p3_payment_orders" }

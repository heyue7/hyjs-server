package db

// LocalWalletReason 保存运营入账文案、金额区间和整数抽取权重。
type LocalWalletReason struct {
	ID       string `gorm:"primaryKey;size:64" json:"id"`
	Label    string `gorm:"size:180;not null" json:"label"`
	MinCents int64  `gorm:"not null" json:"minCents"`
	MaxCents int64  `gorm:"not null" json:"maxCents"`
	Weight   int64  `gorm:"not null" json:"weight"`
	Enabled  bool   `gorm:"not null" json:"enabled"`
}

func (LocalWalletReason) TableName() string { return "local_wallet_reasons" }

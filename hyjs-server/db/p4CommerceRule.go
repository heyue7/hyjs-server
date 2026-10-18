package db

// P4CommerceRule 冻结同一运营实例目录；具体货币来源和未上线玩法联动保持可查。
type P4CommerceRule struct {
	ID         string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	ShopType   int32  `gorm:"not null;index" json:"shopType"`
	Kind       string `gorm:"type:varbinary(32);not null" json:"kind"`
	Enabled    bool   `gorm:"not null" json:"enabled"`
	StartAt    int64  `gorm:"not null" json:"startAt"`
	EndAt      int64  `gorm:"not null" json:"endAt"`
	ClaimEndAt int64  `gorm:"not null" json:"claimEndAt"`
	Rules      []byte `gorm:"type:longblob;not null" json:"rules"`
	Frozen     []byte `gorm:"type:longblob" json:"-"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P4CommerceRule) TableName() string { return "p4_commerce_rules" }

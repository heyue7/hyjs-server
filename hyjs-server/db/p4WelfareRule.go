package db

// P4WelfareRule 保存独立福利窗口与明确的本地奖历，角色首次访问冻结规则。
type P4WelfareRule struct {
	ID         string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	Kind       string `gorm:"type:varbinary(32);not null;index" json:"kind"`
	Enabled    bool   `gorm:"not null" json:"enabled"`
	StartAt    int64  `gorm:"not null" json:"startAt"`
	EndAt      int64  `gorm:"not null" json:"endAt"`
	ClaimEndAt int64  `gorm:"not null" json:"claimEndAt"`
	Rules      []byte `gorm:"type:longblob;not null" json:"rules"`
	Frozen     []byte `gorm:"type:longblob" json:"-"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P4WelfareRule) TableName() string { return "p4_welfare_rules" }

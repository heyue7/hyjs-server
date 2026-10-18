package db

// P4CopyBenefitGrant 保存已确认首次掉落的额外投递，源重放不会再次创建实例。
type P4CopyBenefitGrant struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	SourceKey string `gorm:"primaryKey;type:varbinary(128)" json:"sourceKey"`
	RuleID    string `gorm:"type:varbinary(64);not null" json:"ruleId"`
	BaseHash  string `gorm:"type:char(64);not null" json:"baseHash"`
	Bonus     []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P4CopyBenefitGrant) TableName() string { return "p4_copy_benefit_grants" }

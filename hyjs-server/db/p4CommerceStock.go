package db

// P4CommerceStock 在规则行锁内记录共享库存，与扣款和发奖一起提交。
type P4CommerceStock struct {
	RuleID    string `gorm:"primaryKey;type:varbinary(64)" json:"ruleId"`
	ProductID string `gorm:"primaryKey;type:varbinary(64)" json:"productId"`
	Used      int32  `gorm:"not null" json:"used"`
}

func (P4CommerceStock) TableName() string { return "p4_commerce_stocks" }

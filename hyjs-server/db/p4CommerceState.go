package db

// P4CommerceState 保存角色目录限购、刷新计时和固定折扣；不靠客户端传价格执行交易。
type P4CommerceState struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RuleID   string `gorm:"primaryKey;type:varbinary(64)" json:"ruleId"`
	Payload  []byte `gorm:"type:longblob;not null" json:"payload"`
}

func (P4CommerceState) TableName() string { return "p4_commerce_states" }

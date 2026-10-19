package db

// P7ReRushPolicy 保存明确采用 Re 规则的发布状态，旧冻结战斗仍使用原快照。
type P7ReRushPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P7ReRushPolicy) TableName() string { return "p7_re_rush_policies" }

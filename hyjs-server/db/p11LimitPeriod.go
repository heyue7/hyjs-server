package db

// P11LimitPeriod 保存限时竞技的真实独立周期事实，原周期未知时保持关闭。
type P11LimitPeriod struct {
	ID        string `gorm:"primaryKey;type:varchar(64)" json:"id"`
	StartsAt  int64  `gorm:"not null" json:"starts_at"`
	EndsAt    int64  `gorm:"not null" json:"ends_at"`
	RulesJSON string `gorm:"type:text;not null" json:"rules_json"`
	RulesHash string `gorm:"type:varchar(64);not null" json:"rules_hash"`
	Source    string `gorm:"type:text;not null" json:"source"`
	CreatedAt int64  `gorm:"not null" json:"created_at"`
}

func (P11LimitPeriod) TableName() string { return "p11_limit_periods" }

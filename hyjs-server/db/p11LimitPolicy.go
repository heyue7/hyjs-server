package db

// P11LimitPolicy 保存限时竞技的真实独立周期事实，原周期未知时保持关闭。
type P11LimitPolicy struct {
	ID            int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	RewardEnabled bool   `gorm:"not null" json:"reward_enabled"`
	Enabled       bool   `gorm:"not null" json:"enabled"`
	PeriodID      string `gorm:"type:varchar(64);not null" json:"period_id"`
	StartsAt      int64  `gorm:"not null" json:"starts_at"`
	EndsAt        int64  `gorm:"not null" json:"ends_at"`
	Source        string `gorm:"type:text;not null" json:"source"`
}

func (P11LimitPolicy) TableName() string { return "p11_limit_policies" }

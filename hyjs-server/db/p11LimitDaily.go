package db

// P11LimitDaily 保存限时竞技的真实独立周期事实，原周期未知时保持关闭。
type P11LimitDaily struct {
	PeriodID   string `gorm:"primaryKey;type:varchar(64)" json:"period_id"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day        string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	Used       int32  `gorm:"not null" json:"used"`
	Bought     int32  `gorm:"not null" json:"bought"`
	LoginScore int32  `gorm:"not null" json:"login_score"`
	CreatedAt  int64  `gorm:"not null" json:"created_at"`
}

func (P11LimitDaily) TableName() string { return "p11_limit_daily" }

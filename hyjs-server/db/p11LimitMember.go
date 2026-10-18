package db

// P11LimitMember 保存限时竞技的真实独立周期事实，原周期未知时保持关闭。
type P11LimitMember struct {
	PeriodID  string `gorm:"primaryKey;type:varchar(64)" json:"period_id"`
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Score     int64  `gorm:"not null" json:"score"`
	CreatedAt int64  `gorm:"not null" json:"created_at"`
	UpdatedAt int64  `gorm:"autoUpdateTime:false;not null" json:"updated_at"`
}

func (P11LimitMember) TableName() string { return "p11_limit_members" }

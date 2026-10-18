package db

// P11GrabSession 保存原奖杯争夺空位与实际占领分段，互斥占位不生成战斗赢家。
type P11GrabSession struct {
	ID        string `gorm:"primaryKey;type:varchar(64)" json:"id"`
	PlayerID  int64  `gorm:"index;not null" json:"player_id"`
	Position  int64  `gorm:"not null" json:"position"`
	StartedAt int64  `gorm:"not null" json:"started_at"`
	ExpiresAt int64  `gorm:"not null" json:"expires_at"`
	EndedAt   int64  `gorm:"not null" json:"ended_at"`
	EndReason string `gorm:"type:varchar(32);not null" json:"end_reason"`
}

func (P11GrabSession) TableName() string { return "p11_grab_sessions" }

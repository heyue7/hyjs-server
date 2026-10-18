package db

// P11GrabSlot 保存原奖杯争夺空位与实际占领分段，互斥占位不生成战斗赢家。
type P11GrabSlot struct {
	Position  int64  `gorm:"primaryKey;autoIncrement:false" json:"position"`
	OwnerID   *int64 `gorm:"uniqueIndex:idx_p11_grab_owner" json:"owner_id"`
	SessionID string `gorm:"type:varchar(64);not null" json:"session_id"`
	StartedAt int64  `gorm:"not null" json:"started_at"`
	ExpiresAt int64  `gorm:"not null" json:"expires_at"`
}

func (P11GrabSlot) TableName() string { return "p11_grab_slots" }

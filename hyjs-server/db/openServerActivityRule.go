package db

// OpenServerActivityRule 保存开服活动模板时窗；实际角色起点由PlayerOpeningCalendar投影。
type OpenServerActivityRule struct {
	// UnlockStartAt is runtime-only: the serving realm's Beijing opening date.
	// StartAt/EndAt separately retain this role's complete participation window.
	UnlockStartAt int64  `gorm:"-" json:"unlockStartAt,omitempty"`
	ID            string `gorm:"primaryKey;size:64" json:"id"`
	Enabled       bool   `gorm:"not null" json:"enabled"`
	StartAt       int64  `gorm:"not null" json:"start_at"`
	EndAt         int64  `gorm:"not null" json:"end_at"`
	RewardEndAt   int64  `gorm:"not null" json:"reward_end_at"`
	Source        string `gorm:"type:text;not null" json:"source"`
}

func (OpenServerActivityRule) TableName() string { return "open_server_activity_rules" }

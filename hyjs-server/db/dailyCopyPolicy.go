package db

// DailyCopyPolicy 记录 APK 未提供服务端规则时采用的明确本地策略。
type DailyCopyPolicy struct {
	ID       string `gorm:"primaryKey;size:64" json:"id"`
	Enabled  bool   `gorm:"not null" json:"enabled"`
	RoomID   string `gorm:"size:128;not null" json:"room_id"`
	MaxRound int32  `gorm:"not null" json:"max_round"`
	WinStars int32  `gorm:"not null" json:"win_stars"`
	Source   string `gorm:"type:text;not null" json:"source"`
}

func (DailyCopyPolicy) TableName() string { return "daily_copy_policies" }

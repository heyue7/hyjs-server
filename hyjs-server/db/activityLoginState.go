package db

// ActivityLoginState 记录成功角色登录的UTC+8日期与累计天数，领取状态独立于月签到。
type ActivityLoginState struct {
	ActivityID string `gorm:"primaryKey;size:64" json:"activityId"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	LastDay    string `gorm:"size:10;not null" json:"lastDay"`
	Days       int32  `gorm:"not null" json:"days"`
	Claims     []byte `gorm:"type:longblob" json:"claims"`
}

func (ActivityLoginState) TableName() string { return "activity_login_states" }

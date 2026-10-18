package db

// P13TeamPolicy 冻结具名本地队伍容量5与无时钟过期策略；默认关闭真实变更。
type P13TeamPolicy struct {
	ID              int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	MaxSize         int32  `gorm:"not null" json:"maxSize"`
	InviteTTLMillis int64  `gorm:"not null" json:"inviteTTLMillis"`
	Source          string `gorm:"type:text;not null" json:"source"`
}

func (P13TeamPolicy) TableName() string { return "p13_team_policies" }

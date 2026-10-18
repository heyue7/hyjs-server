package db

// P10ExpeditionHelpPolicy 单独开放实际好友远征救援，不开放缺失事件或伪造风险。
type P10ExpeditionHelpPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null;default:false" json:"enabled"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P10ExpeditionHelpPolicy) TableName() string { return "p10_expedition_help_policies" }

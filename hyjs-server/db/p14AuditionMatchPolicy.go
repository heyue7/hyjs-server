package db

// P14AuditionMatchPolicy 独立控制真实海选计算，初始化保持关闭，权威结算仍核对P6。
type P14AuditionMatchPolicy struct {
	// AutoRecoveryEnabled is a separately prepublished local downtime policy.
	AutoRecoveryEnabled     bool   `gorm:"not null;default:false" json:"auto_recovery_enabled"`
	AutoRecoveryPublishedAt int64  `gorm:"not null;default:0" json:"auto_recovery_published_at"`
	AutoRecoverySource      string `gorm:"type:varchar(1024);not null;default:''" json:"auto_recovery_source"`
	ID                      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled                 bool   `gorm:"not null;default:0" json:"enabled"`
	Source                  string `gorm:"type:text;not null" json:"source"`
}

func (P14AuditionMatchPolicy) TableName() string { return "p14_audition_match_policies" }

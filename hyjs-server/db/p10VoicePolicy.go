package db

// P10VoicePolicy 显式本地WAV适配门禁，原外部SDK认证未恢复，默认关闭。
type P10VoicePolicy struct {
	ID                int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled           bool   `gorm:"not null;default:false" json:"enabled"`
	RetentionMillis   int64  `gorm:"not null;default:0" json:"retention_millis"`
	MinDurationMillis int64  `gorm:"not null;default:0" json:"min_duration_millis"`
	Source            string `gorm:"type:text;not null" json:"source"`
}

func (P10VoicePolicy) TableName() string { return "p10_voice_policies" }

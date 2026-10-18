package db

// P10VoiceMessage 保存真实发送的原isAudio2帧，history与下载仍重新校验当前权限。
type P10VoiceMessage struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	AssetID     string `gorm:"type:char(32);not null;uniqueIndex" json:"asset_id"`
	OwnerID     int64  `gorm:"not null;index" json:"owner_id"`
	TargetID    int64  `gorm:"not null;default:0;index" json:"target_id"`
	ChatType    int64  `gorm:"not null;index" json:"chat_type"`
	Scope       string `gorm:"type:varchar(160);not null;index" json:"scope"`
	Frame       []byte `gorm:"type:longblob;not null" json:"-"`
	FrameHash   string `gorm:"type:char(64);not null" json:"frame_hash"`
	RequestHash string `gorm:"type:char(64);not null" json:"request_hash"`
	CreatedAt   int64  `gorm:"not null;index" json:"created_at"`
}

func (P10VoiceMessage) TableName() string { return "p10_voice_messages" }

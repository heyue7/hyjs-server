package db

// P10VoiceAsset 只有真实WAV上传后才创建opaque filename；音频字节/SHA/会话与范围原子保存。
type P10VoiceAsset struct {
	ID               string `gorm:"primaryKey;type:char(32)" json:"id"`
	FileName         string `gorm:"type:varchar(64);not null;uniqueIndex" json:"file_name"`
	OwnerID          int64  `gorm:"not null;uniqueIndex:p10_voice_upload,priority:1;index" json:"owner_id"`
	RequestID        string `gorm:"type:varchar(64);not null;uniqueIndex:p10_voice_upload,priority:2" json:"request_id"`
	RequestHash      string `gorm:"type:char(64);not null" json:"request_hash"`
	OwnerSessionHash string `gorm:"type:char(64);not null" json:"owner_session_hash"`
	ChatType         int64  `gorm:"not null;index" json:"chat_type"`
	Scope            string `gorm:"type:varchar(160);not null;index" json:"scope"`
	TargetID         int64  `gorm:"not null;default:0" json:"target_id"`
	State            int64  `gorm:"not null" json:"state"`
	Bytes            []byte `gorm:"type:longblob;not null" json:"-"`
	SHA              string `gorm:"type:char(64);not null" json:"sha"`
	ByteLength       int64  `gorm:"not null" json:"byte_length"`
	DurationMillis   int64  `gorm:"not null" json:"duration_millis"`
	CreatedAt        int64  `gorm:"not null" json:"created_at"`
	ExpiresAt        int64  `gorm:"not null;index" json:"expires_at"`
	Source           string `gorm:"type:text;not null" json:"source"`
	MetaHash         string `gorm:"type:char(64);not null" json:"meta_hash"`
}

func (P10VoiceAsset) TableName() string { return "p10_voice_assets" }

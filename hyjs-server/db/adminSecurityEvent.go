package db

// AdminSecurityEvent 保存按分钟聚合的安全事件；EventKey 仅供内部去重，不对外返回。
type AdminSecurityEvent struct {
	ID          int64  `gorm:"primaryKey" json:"id"`
	EventKey    string `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	FirstAt     int64  `gorm:"not null" json:"firstAt"`
	LastAt      int64  `gorm:"not null;index" json:"lastAt"`
	Actor       string `gorm:"size:64;not null" json:"actor"`
	SourceIP    string `gorm:"size:64;not null" json:"sourceIp"`
	Action      string `gorm:"size:64;not null;index" json:"action"`
	Target      string `gorm:"size:191;not null" json:"target"`
	Outcome     string `gorm:"size:16;not null;index" json:"outcome"`
	ReasonCode  string `gorm:"size:64;not null" json:"reasonCode"`
	RequestID   string `gorm:"size:64;not null" json:"requestId"`
	Occurrences int64  `gorm:"not null" json:"occurrences"`
}

func (AdminSecurityEvent) TableName() string { return "admin_security_events" }

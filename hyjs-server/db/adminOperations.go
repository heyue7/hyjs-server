package db

// AdminAudit 保存已提交后台变更的操作者和有限差量摘要。
type AdminAudit struct {
	ID     int64  `gorm:"primaryKey" json:"id"`
	Time   int64  `gorm:"not null;index" json:"time"`
	Actor  string `gorm:"size:64;not null" json:"actor"`
	Action string `gorm:"size:64;not null" json:"action"`
	Target string `gorm:"size:191;not null" json:"target"`
	Detail string `gorm:"type:text;not null" json:"detail"`
}

func (AdminAudit) TableName() string { return "admin_audits" }

// AdminAnnouncement 为原客户端公告页提供纯文本发布内容。
type AdminAnnouncement struct {
	ID              int64  `gorm:"primaryKey" json:"id"`
	Title           string `gorm:"size:160;not null" json:"title"`
	Content         string `gorm:"type:text;not null" json:"content"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	StartAt         int64  `gorm:"not null" json:"startAt"`
	EndAt           int64  `gorm:"not null" json:"endAt"`
	UpdatedAtMillis int64  `gorm:"not null" json:"updatedAt"`
	Version         string `gorm:"-" json:"version"`
}

func (AdminAnnouncement) TableName() string { return "admin_announcements" }

// AdminGrantReceipt 与原附件同事务提交，重复请求不能增加发放。
type AdminGrantReceipt struct {
	RequestID  string `gorm:"primaryKey;size:64" json:"requestId"`
	ActorID    int64  `gorm:"not null" json:"actorId"`
	BodyHash   string `gorm:"size:64;not null" json:"-"`
	DeliveryID int64  `gorm:"not null" json:"deliveryId"`
}

func (AdminGrantReceipt) TableName() string { return "admin_grant_receipts" }

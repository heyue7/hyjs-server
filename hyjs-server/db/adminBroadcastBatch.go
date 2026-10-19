package db

// AdminBroadcastBatch freezes an operator preview before any original mail is sent.
type AdminBroadcastBatch struct {
	ID              string  `gorm:"primaryKey;size:36" json:"id"`
	ServerID        int64   `gorm:"not null;index" json:"serverId"`
	ActorID         int64   `gorm:"not null;index" json:"actorId"`
	ActorName       string  `gorm:"size:80;not null" json:"actorName"`
	Title           string  `gorm:"size:191;not null" json:"title"`
	Body            string  `gorm:"type:text;not null" json:"body"`
	Rewards         string  `gorm:"type:text;not null" json:"-"`
	Reason          string  `gorm:"type:text;not null" json:"reason"`
	BodyHash        string  `gorm:"size:64;not null" json:"-"`
	Version         string  `gorm:"size:64;not null" json:"version"`
	RecipientCount  int64   `gorm:"not null" json:"recipientCount"`
	ExcludedCount   int64   `gorm:"not null" json:"excludedCount"`
	SuccessCount    int64   `gorm:"not null" json:"successCount"`
	CreatedAt       int64   `gorm:"not null;autoCreateTime:false" json:"createdAt"`
	ExpiresAt       int64   `gorm:"not null" json:"expiresAt"`
	SentAt          int64   `gorm:"not null" json:"sentAt"`
	CommitRequestID *string `gorm:"size:36;uniqueIndex:admin_broadcast_commit" json:"-"`
}

func (AdminBroadcastBatch) TableName() string { return "admin_broadcast_batches" }

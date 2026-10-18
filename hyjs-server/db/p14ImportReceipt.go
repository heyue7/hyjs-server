package db

// P14ImportReceipt 保存每个签名跨服单号及原载荷hash，断网确认重试仍返回同一次导入。
type P14ImportReceipt struct {
	ID             string `gorm:"primaryKey;type:char(64)" json:"id"`
	PayloadHash    string `gorm:"type:char(64);not null" json:"payloadHash"`
	SourceServerID int64  `gorm:"not null" json:"sourceServerId"`
	ImportedAt     int64  `gorm:"not null" json:"importedAt"`
}

func (P14ImportReceipt) TableName() string { return "p14_import_receipts" }

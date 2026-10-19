package db

// AdminCDKCode stores operator policy beside the original hashed gift-code ledger.
// Plain redemption codes are returned only during generation and never stored.
type AdminCDKCode struct {
	Hash      string `gorm:"primaryKey;size:64"`
	ServerID  int64  `gorm:"not null;index"`
	Name      string `gorm:"size:80;not null"`
	CodeHint  string `gorm:"size:16;not null"`
	Enabled   bool   `gorm:"not null"`
	Unlimited bool   `gorm:"not null"`
	StartAt   int64  `gorm:"not null"`
	EndAt     int64  `gorm:"not null"`
	Revision  int64  `gorm:"not null"`
	CreatedAt int64  `gorm:"not null;autoCreateTime:false"`
	UpdatedAt int64  `gorm:"not null;autoUpdateTime:false"`
}

func (AdminCDKCode) TableName() string { return "admin_cdk_codes" }

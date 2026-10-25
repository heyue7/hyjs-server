package db

// P7RushHistoryCertificate is derived verification evidence. Actual frozen rows
// and their complete fingerprint remain authoritative on every query.
type P7RushHistoryCertificate struct {
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false"`
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false"`
	FightID     string `gorm:"primaryKey;type:varbinary(64)"`
	Format      int64  `gorm:"not null"`
	Validator   string `gorm:"type:char(64);not null"`
	Fingerprint string `gorm:"type:char(64);not null"`
	OutcomeHash string `gorm:"type:char(64);not null"`
	VerifiedAt  int64  `gorm:"not null"`
	MAC         string `gorm:"column:mac;type:char(64);not null"`
}

func (P7RushHistoryCertificate) TableName() string { return "p7_rush_history_certificates" }

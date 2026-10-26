package db

// P7RushHistoryCertificate durably proves an accepted settled fact under the
// explicit validation contract. Every query binds all actual frozen source bytes.
type P7RushHistoryCertificate struct {
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false"`
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false"`
	FightID     string `gorm:"primaryKey;type:varbinary(64)"`
	Format      int64  `gorm:"not null"`
	Validator   string `gorm:"type:char(64);not null"`
	Fingerprint string `gorm:"type:char(64);not null"`
	Winner      int32  `gorm:"not null;default:0"`
	OutcomeHash string `gorm:"type:char(64);not null"`
	VerifiedAt  int64  `gorm:"not null"`
	MAC         string `gorm:"column:mac;type:char(64);not null"`
}

func (P7RushHistoryCertificate) TableName() string { return "p7_rush_history_certificates" }

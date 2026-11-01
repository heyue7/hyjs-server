package db

// CopySweepWitness keeps one role's latest response across old-binary rewrites.
// The maximum digest also detects an inconsistent legacy profile watermark.
type CopySweepWitness struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false;not null"`
	MaxStamp     int64  `gorm:"not null"`
	MaxDigest    []byte `gorm:"type:binary(32);not null"`
	LastStamp    int64  `gorm:"not null"`
	LastDigest   []byte `gorm:"type:binary(32);not null"`
	LastRequest  []byte `gorm:"type:longblob;not null"`
	LastResponse []byte `gorm:"type:longblob;not null"`
}

func (CopySweepWitness) TableName() string { return "copy_sweep_witnesses" }

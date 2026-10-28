package db

// GGOFieldReceipt retains one immutable V4 action, settlement, death or trade fact.
// A nullable business key prevents the same token from paying twice across request UUIDs.
type GGOFieldReceipt struct {
	PlayerID       int64   `gorm:"primaryKey;autoIncrement:false;uniqueIndex:uidx_ggo_field_business,priority:1"`
	RequestID      string  `gorm:"primaryKey;type:varbinary(36)"`
	Operation      string  `gorm:"type:varbinary(32);not null;uniqueIndex:uidx_ggo_field_business,priority:2;index:idx_ggo_field_due,priority:1"`
	BusinessKey    *string `gorm:"type:varbinary(128);uniqueIndex:uidx_ggo_field_business,priority:3"`
	RequestHash    string  `gorm:"type:char(64);not null"`
	Response       []byte  `gorm:"type:longblob;not null"`
	CounterpartyID int64   `gorm:"not null"`
	EconomyVersion string  `gorm:"type:varbinary(64);not null"`
	ChipDelta      []byte  `gorm:"type:blob;not null"`
	CreatedAt      int64   `gorm:"not null;autoCreateTime:false"`
	Active         bool    `gorm:"not null;default:false;index:idx_ggo_field_due,priority:2"`
	ReadyAt        int64   `gorm:"not null;default:0;index:idx_ggo_field_due,priority:3"`
}

func (GGOFieldReceipt) TableName() string { return "ggo_field_receipts" }

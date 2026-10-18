package db

// TurntableDrawReceipt preserves every committed private draw across schedule changes.
type TurntableDrawReceipt struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Stamp    int64  `gorm:"primaryKey;autoIncrement:false" json:"-"`
	PoolID   string `gorm:"size:64;not null" json:"-"`
	Count    int32  `gorm:"not null" json:"-"`
	Response []byte `gorm:"type:longblob;not null" json:"-"`
}

func (TurntableDrawReceipt) TableName() string { return "turntable_draw_receipts" }

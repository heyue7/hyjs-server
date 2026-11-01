package db

// P8ResourceReservation serializes only one field point and line. Line zero
// is the per-point guard and also reserves all lines when presence is unknown.
type P8ResourceReservation struct {
	ServerID      int64  `gorm:"primaryKey;autoIncrement:false"`
	Scene         string `gorm:"primaryKey;size:64"`
	PosID         string `gorm:"primaryKey;size:64"`
	Line          int32  `gorm:"primaryKey;autoIncrement:false"`
	PlayerID      int64  `gorm:"not null;default:0"`
	Token         string `gorm:"size:64;not null;default:''"`
	OccupiedUntil int64  `gorm:"not null;default:0"`
	ReplayGuard   bool   `gorm:"not null;default:false"`
}

func (P8ResourceReservation) TableName() string { return "p8_resource_reservations" }

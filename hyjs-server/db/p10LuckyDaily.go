package db

// P10LuckyDaily 保存UTC+8原日次数和TCP无时间戳投放的有限CD重传回执。
type P10LuckyDaily struct {
	PlayerID        int64  `gorm:"primaryKey;autoIncrement:false"`
	Day             string `gorm:"primaryKey;type:varchar(10)"`
	FireworksUsed   int32  `gorm:"not null"`
	DropBatches     int32  `gorm:"not null"`
	Snatched        int32  `gorm:"not null"`
	FireworksAt     int64  `gorm:"not null"`
	DropAt          int64  `gorm:"not null"`
	DropRequestHash string `gorm:"type:varchar(64);not null"`
	DropResponse    []byte `gorm:"type:longblob"`
}

func (P10LuckyDaily) TableName() string { return "p10_lucky_dailies" }

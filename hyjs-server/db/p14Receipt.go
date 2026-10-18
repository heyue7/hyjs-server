package db

// P14Receipt 将原变更帧精确绑定到原玩家、操作和stamp，重试不再上传新版本。
type P14Receipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Operation string `gorm:"primaryKey;type:varchar(64)" json:"operation"`
	Stamp     int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	Request   []byte `gorm:"type:longblob;not null" json:"-"`
	Response  []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P14Receipt) TableName() string { return "p14_receipts" }

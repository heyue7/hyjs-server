package db

// P13TeamReceipt 保留原请求和响应，不允许stamp重试创建第二队或重复推送。
type P13TeamReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Operation string `gorm:"primaryKey;type:varchar(64)" json:"operation"`
	Stamp     int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	Request   []byte `gorm:"type:longblob;not null" json:"-"`
	Response  []byte `gorm:"type:longblob;not null" json:"-"`
	CreatedAt int64  `gorm:"not null" json:"createdAt"`
}

func (P13TeamReceipt) TableName() string { return "p13_team_receipts" }

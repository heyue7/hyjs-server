package db

// P13GuildReceipt 保存精确请求及原二进制响应，重试不重复扣费或变更职位。
type P13GuildReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Operation string `gorm:"primaryKey;type:varchar(64)" json:"operation"`
	Stamp     int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	Request   []byte `gorm:"type:longblob;not null" json:"-"`
	Response  []byte `gorm:"type:longblob;not null" json:"-"`
	CreatedAt int64  `gorm:"not null" json:"createdAt"`
}

func (P13GuildReceipt) TableName() string { return "p13_guild_receipts" }

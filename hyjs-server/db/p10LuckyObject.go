package db

// P10LuckyObject 保存真实同线对象、已扣来源、冻结奖项和原领取回包。
type P10LuckyObject struct {
	GUID          string `gorm:"primaryKey;type:varchar(64)"`
	OwnerID       int64  `gorm:"not null;index"`
	OwnerName     string `gorm:"type:varchar(128);not null"`
	Scene         string `gorm:"type:varchar(64);not null;index:idx_p10_lucky_scope,priority:1"`
	Line          int32  `gorm:"not null;index:idx_p10_lucky_scope,priority:2"`
	CreatedAt     int64  `gorm:"not null"`
	ExpiresAt     int64  `gorm:"not null;index:idx_p10_lucky_scope,priority:3"`
	Frame         []byte `gorm:"type:longblob;not null"`
	Reward        string `gorm:"type:varchar(512);not null"`
	RewardHash    string `gorm:"type:varchar(64);not null"`
	ClaimedBy     int64  `gorm:"not null"`
	ClaimedAt     int64  `gorm:"not null"`
	ClaimResponse []byte `gorm:"type:longblob"`
}

func (P10LuckyObject) TableName() string { return "p10_lucky_objects" }

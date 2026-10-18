package db

// P10LuckyPolicy 透明有限运营门禁及来自原表的完整57项SQL奖池。
type P10LuckyPolicy struct {
	ID               int64  `gorm:"primaryKey;autoIncrement:false"`
	FireworksEnabled bool   `gorm:"not null"`
	DropEnabled      bool   `gorm:"not null"`
	DropDailyBatches int32  `gorm:"not null"`
	FireworksIndex   int32  `gorm:"not null"`
	RewardJSON       string `gorm:"type:longtext;not null"`
	RewardHash       string `gorm:"type:varchar(64);not null"`
	Source           string `gorm:"type:text;not null"`
}

func (P10LuckyPolicy) TableName() string { return "p10_lucky_policies" }

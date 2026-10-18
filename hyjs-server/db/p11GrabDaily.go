package db

// P11GrabDaily 保存原奖杯争夺空位与实际占领分段，互斥占位不生成战斗赢家。
type P11GrabDaily struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Day         string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	RewardsJSON string `gorm:"type:text;not null" json:"rewards_json"`
	RewardsHash string `gorm:"type:varchar(64);not null" json:"rewards_hash"`
	Used        int32  `gorm:"not null" json:"used"`
	Bought      int32  `gorm:"not null" json:"bought"`
}

func (P11GrabDaily) TableName() string { return "p11_grab_daily" }

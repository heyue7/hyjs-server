package db

// P10FriendEnergyDaily 冻结好友耐力真实来源与每日领取事实。
type P10FriendEnergyDaily struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false"`
	Day      string `gorm:"primaryKey;type:char(10)"`
	Claimed  int32  `gorm:"not null"`
}

func (P10FriendEnergyDaily) TableName() string { return "p10_friend_energy_daily" }

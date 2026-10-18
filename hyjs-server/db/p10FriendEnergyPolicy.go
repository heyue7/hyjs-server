package db

// P10FriendEnergyPolicy 冻结好友耐力真实来源与每日领取事实。
type P10FriendEnergyPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false"`
	Enabled bool   `gorm:"not null"`
	Amount  int32  `gorm:"not null"`
	Source  string `gorm:"type:text;not null"`
}

func (P10FriendEnergyPolicy) TableName() string { return "p10_friend_energy_policies" }

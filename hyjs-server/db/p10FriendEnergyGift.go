package db

// P10FriendEnergyGift 冻结好友耐力真实来源与每日领取事实。
type P10FriendEnergyGift struct {
	SenderID    int64  `gorm:"primaryKey;autoIncrement:false"`
	ReceiverID  int64  `gorm:"primaryKey;autoIncrement:false"`
	Day         string `gorm:"primaryKey;type:char(10)"`
	Amount      int32  `gorm:"not null"`
	FriendSince int64  `gorm:"not null"`
	Invalidated bool   `gorm:"not null"`
	Claimed     bool   `gorm:"not null"`
	CreatedAt   int64  `gorm:"not null"`
}

func (P10FriendEnergyGift) TableName() string { return "p10_friend_energy_gifts" }

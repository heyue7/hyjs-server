package db

// P10ChannelPolicy 分别控制原世界与第一野外文字频道的新消息，默认关闭。
type P10ChannelPolicy struct {
	ID           int64  `gorm:"primaryKey;autoIncrement:false"`
	WorldEnabled bool   `gorm:"not null"`
	FieldEnabled bool   `gorm:"not null"`
	Source       string `gorm:"type:text;not null"`
}

func (P10ChannelPolicy) TableName() string { return "p10_channel_policies" }

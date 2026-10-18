package db

// P13UnionFightChatPolicy 独立开放场内文字与原18指挥；指挥冷却为明确本地参数。
type P13UnionFightChatPolicy struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	TextEnabled        bool   `gorm:"not null;default:false" json:"textEnabled"`
	TalkEnabled        bool   `gorm:"not null;default:false" json:"talkEnabled"`
	TalkCooldownMillis int64  `gorm:"not null;default:20000" json:"talkCooldownMillis"`
	Source             string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightChatPolicy) TableName() string { return "p13_union_fight_chat_policies" }

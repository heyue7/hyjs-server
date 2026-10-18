package db

// P13UnionFightChat 保存本场本会实际文字或指挥原帧，不向新比赛/外会复用。
type P13UnionFightChat struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	MatchID   string `gorm:"type:char(64);not null;index:p13_uf_chat_scope,priority:1" json:"matchId"`
	GuildID   string `gorm:"type:varchar(64);not null;index:p13_uf_chat_scope,priority:2" json:"guildId"`
	Kind      int64  `gorm:"not null;index:p13_uf_chat_scope,priority:3" json:"kind"`
	SenderID  int64  `gorm:"not null;index:p13_uf_chat_sender,priority:1" json:"senderId"`
	CreatedAt int64  `gorm:"not null;index:p13_uf_chat_sender,priority:2" json:"createdAt"`
	Request   []byte `gorm:"type:blob;not null" json:"-"`
	Frame     []byte `gorm:"type:blob;not null" json:"-"`
	FrameHash string `gorm:"type:char(64);not null" json:"frameHash"`
	Source    string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightChat) TableName() string { return "p13_union_fight_chats" }

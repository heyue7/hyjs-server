package db

// P13UnionFightFocus 保存本场本会唯一原大地图集结点、真实指挥者、绝对到期及冷却；不授予战斗/传送效果。
type P13UnionFightFocus struct {
	MatchID     string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	GuildID     string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	LeaderID    int64  `gorm:"not null" json:"leaderId"`
	Point       []byte `gorm:"type:blob;not null" json:"-"`
	PointHash   string `gorm:"type:char(64);not null" json:"pointHash"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
	ExpiresAt   int64  `gorm:"not null" json:"expiresAt"`
	ReadyAt     int64  `gorm:"not null" json:"readyAt"`
	ClearedAt   int64  `gorm:"not null;default:0" json:"clearedAt"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightFocus) TableName() string { return "p13_union_fight_focus" }

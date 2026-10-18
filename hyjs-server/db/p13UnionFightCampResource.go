package db

// P13UnionFightCampResource 保存当场公会已实际上交的矿石；普通公会金币/个人贡献不在此表。
type P13UnionFightCampResource struct {
	MatchID string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	GuildID string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	Camp    int64  `gorm:"not null" json:"camp"`
	Amount  int64  `gorm:"not null;default:0" json:"amount"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightCampResource) TableName() string { return "p13_union_fight_camp_resources" }

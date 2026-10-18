package db

// P13UnionFightGroup 保留本周首次分组与冻结战力，后续报名不重分已有公会。
type P13UnionFightGroup struct {
	WeekStart   string `gorm:"primaryKey;type:varchar(10)" json:"weekStart"`
	GuildID     string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	Group       string `gorm:"type:char(1);not null" json:"group"`
	FrozenPower int64  `gorm:"not null" json:"frozenPower"`
	AssignedAt  int64  `gorm:"not null" json:"assignedAt"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightGroup) TableName() string { return "p13_union_fight_groups" }

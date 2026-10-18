package db

// P13UnionFightFighter 保留宣战时名册职位与报名时真实阵容；名单冻结后不读取现时改装。
type P13UnionFightFighter struct {
	GuildID           string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	Day               string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	PlayerID          int64  `gorm:"primaryKey" json:"playerId"`
	Role              int32  `gorm:"not null" json:"role"`
	AppliedAt         int64  `gorm:"not null;default:0" json:"appliedAt"`
	FirstOn           bool   `gorm:"not null;default:false" json:"firstOn"`
	AutoFight         bool   `gorm:"not null;default:false" json:"autoFight"`
	Name              string `gorm:"type:varchar(128);not null" json:"name"`
	FightPower        int64  `gorm:"not null;default:0" json:"fightPower"`
	Battle            []byte `gorm:"type:longblob" json:"-"`
	SnapshotHash      string `gorm:"type:char(64);not null;default:''" json:"snapshotHash"`
	UpdatedAt         int64  `gorm:"not null;default:0" json:"updatedAt"`
	BattleAdjustTimes int32  `gorm:"not null;default:0" json:"battleAdjustTimes"`
	DefaultItems      string `gorm:"type:varchar(512);not null;default:''" json:"defaultItems"`
}

func (P13UnionFightFighter) TableName() string { return "p13_union_fight_fighters" }

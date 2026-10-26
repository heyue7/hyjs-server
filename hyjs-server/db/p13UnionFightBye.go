package db

// P13UnionFightBye is an actual unmatched round, never a synthetic opponent or battlefield.
type P13UnionFightBye struct {
	ID           string `gorm:"primaryKey;type:char(64)" json:"id"`
	GuildID      string `gorm:"type:varchar(64);not null;uniqueIndex:p13_uf_bye_round,priority:1" json:"guildId"`
	Day          string `gorm:"type:char(10);not null;uniqueIndex:p13_uf_bye_round,priority:2;index:p13_uf_bye_period,priority:1" json:"day"`
	Round        int32  `gorm:"not null;uniqueIndex:p13_uf_bye_round,priority:3" json:"round"`
	Group        string `gorm:"type:char(1);not null;index:p13_uf_bye_period,priority:2" json:"group"`
	StartAt      int64  `gorm:"not null" json:"startAt"`
	EndAt        int64  `gorm:"not null" json:"endAt"`
	CreatedAt    int64  `gorm:"not null" json:"createdAt"`
	WinPrestige  int32  `gorm:"not null" json:"winPrestige"`
	Snapshot     []byte `gorm:"type:longblob;not null" json:"-"`
	SnapshotHash string `gorm:"type:char(64);not null" json:"snapshotHash"`
	SourceHash   string `gorm:"type:char(64);not null" json:"sourceHash"`
	Source       string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightBye) TableName() string { return "p13_union_fight_byes" }

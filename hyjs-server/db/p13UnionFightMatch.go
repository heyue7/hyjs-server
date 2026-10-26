package db

// P13UnionFightMatch 真实公会配对与原预览按场次固定，无敌人时保留待配对，不写胜负。
type P13UnionFightMatch struct {
	ID                string `gorm:"primaryKey;type:char(64);index:p13_uf_match_terminal_cursor,priority:2;index:p13_uf_match_start_cursor,priority:2" json:"id"`
	Day               string `gorm:"type:varchar(10);not null;index" json:"day"`
	Group             string `gorm:"type:char(1);not null" json:"group"`
	Round             int32  `gorm:"not null" json:"round"`
	LeftGuildID       string `gorm:"type:varchar(64);not null;index" json:"leftGuildId"`
	RightGuildID      string `gorm:"type:varchar(64);not null;index" json:"rightGuildId"`
	StartAt           int64  `gorm:"not null;index:p13_uf_match_start_cursor,priority:1" json:"startAt"`
	EndAt             int64  `gorm:"not null;index:p13_uf_match_terminal_cursor,priority:1" json:"endAt"`
	LeftPreview       []byte `gorm:"type:longblob;not null" json:"-"`
	RightPreview      []byte `gorm:"type:longblob;not null" json:"-"`
	Roster            []byte `gorm:"type:longblob;not null" json:"-"`
	SnapshotHash      string `gorm:"type:char(64);not null" json:"snapshotHash"`
	ConfigurationHash string `gorm:"type:char(64);not null" json:"configurationHash"`
	EngineVersion     string `gorm:"type:varchar(128);not null" json:"engineVersion"`
	ClientVersion     string `gorm:"type:varchar(128);not null" json:"clientVersion"`
	CreatedAt         int64  `gorm:"not null" json:"createdAt"`
	Source            string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightMatch) TableName() string { return "p13_union_fight_matches" }

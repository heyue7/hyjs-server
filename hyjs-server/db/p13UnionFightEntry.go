package db

// P13UnionFightEntry 以公会与宣战日唯一保存宣战和冻结截止，不提前创建胜负。
type P13UnionFightEntry struct {
	GuildID           string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	Day               string `gorm:"primaryKey;type:varchar(10);index:p13_uf_entry_due,priority:1" json:"day"`
	AppliedAt         int64  `gorm:"not null" json:"appliedAt"`
	Deadline          int64  `gorm:"not null;index:p13_uf_entry_due,priority:2" json:"deadline"`
	FrozenAt          int64  `gorm:"not null;default:0;index:p13_uf_entry_due,priority:3" json:"frozenAt"`
	EligibleCount     int32  `gorm:"not null;default:0" json:"eligibleCount"`
	AdjustTimesUsed   int32  `gorm:"not null;default:0" json:"adjustTimesUsed"`
	MatchProcessed    bool   `gorm:"not null;default:false;index:p13_uf_entry_due,priority:4" json:"matchProcessed"`
	FrozenTables      []byte `gorm:"type:longblob" json:"-"`
	ConfigurationHash string `gorm:"type:char(64);not null;default:''" json:"configurationHash"`
	EngineVersion     string `gorm:"type:varchar(128);not null;default:''" json:"engineVersion"`
	ClientVersion     string `gorm:"type:varchar(128);not null;default:''" json:"clientVersion"`
	Source            string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightEntry) TableName() string { return "p13_union_fight_entries" }

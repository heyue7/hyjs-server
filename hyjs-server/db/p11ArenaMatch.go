package db

// P11ArenaMatch 保存真实双方当次队伍、名次和配置引用；重复查询与恢复不读取后来改装结果。
type P11ArenaMatch struct {
	ID                string `gorm:"primaryKey;type:varchar(64)"`
	ViewerID          int64  `gorm:"not null;index:idx_p11_match_owner,priority:1"`
	OpponentID        int64  `gorm:"not null;index"`
	BatchID           string `gorm:"type:varchar(64);not null;index:idx_p11_match_owner,priority:2"`
	ViewerRank        int64  `gorm:"not null"`
	OpponentRank      int64  `gorm:"not null"`
	Attacker          []byte `gorm:"type:longblob;not null"`
	Defender          []byte `gorm:"type:longblob;not null"`
	SnapshotHash      string `gorm:"type:varchar(64);not null"`
	ConfigurationHash string `gorm:"type:varchar(64);not null"`
	CapabilityHash    string `gorm:"type:varchar(64);not null"`
	Seed              int64  `gorm:"not null"`
	CreatedAt         int64  `gorm:"not null"`
	ExpiresAt         int64  `gorm:"not null;index"`
}

func (P11ArenaMatch) TableName() string { return "p11_arena_matches" }

package db

// P14AuditionBoundary 与完整本服名册同事务封存截止事实，空名单也明确留档。
type P14AuditionBoundary struct {
	SeasonIndex   int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	RegisterStart int64  `gorm:"not null" json:"register_start"`
	RegisterEnd   int64  `gorm:"not null" json:"register_end"`
	FrozenAt      int64  `gorm:"not null" json:"frozen_at"`
	Count         int64  `gorm:"not null" json:"count"`
	Calendar      []byte `gorm:"type:longblob;not null" json:"calendar"`
	CalendarHash  string `gorm:"type:char(64);not null" json:"calendar_hash"`
	RosterHash    string `gorm:"type:char(64);not null" json:"roster_hash"`
	Source        string `gorm:"type:text;not null" json:"source"`
	SourceHash    string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P14AuditionBoundary) TableName() string { return "p14_audition_boundaries" }

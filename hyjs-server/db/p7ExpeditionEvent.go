package db

// P7ExpeditionEvent 保存实际派遣时冻结的剧情事件，不从好友请求制造事件。
type P7ExpeditionEvent struct {
	Serial        string `gorm:"primaryKey;type:varbinary(64)"`
	MissionSerial string `gorm:"type:varbinary(64);not null;index:idx_p7_exp_event_owner,priority:2"`
	PlayerID      int64  `gorm:"not null;index:idx_p7_exp_event_owner,priority:1"`
	StoryID       string `gorm:"size:64;not null"`
	Kind          int32  `gorm:"not null"`
	EventAt       int64  `gorm:"not null"`
	Reward        string `gorm:"type:text;not null"`
	ResolvedAt    int64  `gorm:"not null"`
	HelperID      int64  `gorm:"not null"`
	ParentSerial  string `gorm:"type:varbinary(64);not null"`
	SourceHash    string `gorm:"size:64;not null"`
}

func (P7ExpeditionEvent) TableName() string { return "p7_expedition_events" }

package db

// P13UnionFightFactoryState 保存本场已冻结来源、真实开启消费和首辆战车的持久生产状态。
type P13UnionFightFactoryState struct {
	// CarGeneration 仅领域内投影后续车快照，不改变厂主键或原协议FactoryGuid。
	CarGeneration int64   `gorm:"-" json:"-"`
	MatchID       string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	GUID          int64   `gorm:"primaryKey;autoIncrement:false" json:"guid"`
	FactoryID     int64   `gorm:"not null" json:"factory_id"`
	Camp          int64   `gorm:"not null" json:"camp"`
	ChariotHP     float64 `gorm:"not null" json:"chariot_hp"`
	Source        string  `gorm:"type:text;not null" json:"source"`
	SourceHash    string  `gorm:"type:char(64);not null" json:"source_hash"`
	Frame         []byte  `gorm:"type:longblob;not null" json:"frame"`
	FrameHash     string  `gorm:"type:char(64);not null" json:"frame_hash"`
	StateHash     string  `gorm:"type:char(64);not null" json:"state_hash"`
	OpenedAt      int64   `gorm:"not null;default:0" json:"opened_at"`
	StartedAt     int64   `gorm:"not null;default:0" json:"started_at"`
	AnimationAt   int64   `gorm:"not null;default:0;index:p13_factory_pending,priority:3" json:"animation_at"`
	DueAt         int64   `gorm:"not null;default:0;index:p13_factory_pending,priority:2" json:"due_at"`
	// CompletedAt 0=待生产，-1=越过合法比赛窗口或公会撤销后归档，正数=原合法出车时刻。
	CompletedAt int64  `gorm:"not null;default:0;index:p13_factory_pending,priority:1" json:"completed_at"`
	CarFrame    []byte `gorm:"type:longblob" json:"car_frame"`
	CarHash     string `gorm:"type:char(64);not null;default:''" json:"car_hash"`
}

func (P13UnionFightFactoryState) TableName() string { return "p13_union_fight_factory_states" }

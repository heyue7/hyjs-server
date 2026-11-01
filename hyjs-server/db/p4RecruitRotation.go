package db

// P4RecruitRotation 保存普通/强者完整轮换窗口，冻结后的预览和实际卡池使用同一数组。
type P4RecruitRotation struct {
	ID      string `gorm:"primaryKey;type:varbinary(64);index:idx_p4_recruit_kind_window,priority:4" json:"id"`
	Kind    int32  `gorm:"not null;index;index:idx_p4_recruit_kind_window,priority:1" json:"kind"`
	Enabled bool   `gorm:"not null;index:idx_p4_recruit_kind_window,priority:2" json:"enabled"`
	StartAt int64  `gorm:"not null;index:idx_p4_recruit_kind_window,priority:3" json:"startAt"`
	EndAt   int64  `gorm:"not null" json:"endAt"`
	Rewards []byte `gorm:"type:longblob;not null" json:"rewards"`
	Frozen  []byte `gorm:"type:longblob" json:"-"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P4RecruitRotation) TableName() string { return "p4_recruit_rotations" }

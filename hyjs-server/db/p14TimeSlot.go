package db

// P14TimeSlot 保存运营明确发布的原阶段时间；同赛季不重叠，不由测试时刻初始化生产日历。
type P14TimeSlot struct {
	SeasonIndex int32  `gorm:"primaryKey;autoIncrement:false" json:"seasonIndex"`
	StartAt     int64  `gorm:"primaryKey;autoIncrement:false" json:"startAt"`
	Stage       int32  `gorm:"not null" json:"stage"`
	EndAt       int64  `gorm:"not null" json:"endAt"`
	Description string `gorm:"type:text;not null" json:"description"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P14TimeSlot) TableName() string { return "p14_time_slots" }

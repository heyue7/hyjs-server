package db

// P13UnionFightSchedule 保存运营明确发布的两场日历；默认没有日历，不伪造原开赛时刻。
type P13UnionFightSchedule struct {
	Day            string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	FirstStartAt   int64  `gorm:"not null" json:"firstStartAt"`
	SecondStartAt  int64  `gorm:"not null" json:"secondStartAt"`
	DurationMillis int64  `gorm:"not null" json:"durationMillis"`
	Source         string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightSchedule) TableName() string { return "p13_union_fight_schedules" }

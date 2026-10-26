package db

// P13UnionFightSchedule 保存运营明确发布的两场日历；默认没有日历，不伪造原开赛时刻。
type P13UnionFightSchedule struct {
	PreparationMillis int64  `gorm:"not null;default:0" json:"preparationMillis"`
	Enabled           bool   `gorm:"not null;default:false;index:idx_p13_uf_calendar,priority:1" json:"enabled"`
	Version           string `gorm:"type:varchar(64);not null;default:''" json:"version"`
	PublishedAt       int64  `gorm:"not null;default:0" json:"publishedAt"`
	RegistrationDay   string `gorm:"type:varchar(10);not null;default:'';index" json:"registrationDay"`
	EnrollmentStartAt int64  `gorm:"not null;default:0;index:idx_p13_uf_calendar,priority:2" json:"enrollmentStartAt"`
	EnrollmentEndAt   int64  `gorm:"not null;default:0" json:"enrollmentEndAt"`
	Day               string `gorm:"primaryKey;type:varchar(10)" json:"day"`
	FirstStartAt      int64  `gorm:"not null" json:"firstStartAt"`
	SecondStartAt     int64  `gorm:"not null" json:"secondStartAt"`
	DurationMillis    int64  `gorm:"not null" json:"durationMillis"`
	Source            string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightSchedule) TableName() string { return "p13_union_fight_schedules" }

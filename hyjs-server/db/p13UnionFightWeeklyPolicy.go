package db

// P13UnionFightWeeklyPolicy publishes named local weekly rewards before the first match.
type P13UnionFightWeeklyPolicy struct {
	WeekStart   string `gorm:"primaryKey;type:char(10)" json:"week_start"`
	Group       string `gorm:"primaryKey;type:char(1)" json:"group"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Version     string `gorm:"type:varchar(64);not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
	Rewards     string `gorm:"type:text;not null" json:"rewards"`
	PublishedAt int64  `gorm:"not null;autoCreateTime:false" json:"published_at"`
}

func (P13UnionFightWeeklyPolicy) TableName() string { return "p13_union_fight_weekly_policies" }

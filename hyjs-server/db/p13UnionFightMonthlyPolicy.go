package db

// P13UnionFightMonthlyPolicy is an explicitly published local future-month rule.
// No historical private reward schedule is seeded or inferred.
type P13UnionFightMonthlyPolicy struct {
	Month         string `gorm:"primaryKey;type:char(7)" json:"month"`
	Enabled       bool   `gorm:"not null;default:false" json:"enabled"`
	Version       string `gorm:"type:varchar(64);not null" json:"version"`
	Source        string `gorm:"type:text;not null" json:"source"`
	PublishedAt   int64  `gorm:"not null;autoCreateTime:false" json:"published_at"`
	Rewards       string `gorm:"type:text;not null" json:"rewards"`
	MemberTitle   bool   `gorm:"not null;default:false" json:"member_title"`
	ChairmanTitle bool   `gorm:"not null;default:false" json:"chairman_title"`
}

func (P13UnionFightMonthlyPolicy) TableName() string { return "p13_union_fight_monthly_policies" }

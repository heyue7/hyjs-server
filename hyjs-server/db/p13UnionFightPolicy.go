package db

// P13UnionFightPolicy 保存独立报名门禁；胜负与匹配仍由后续权威能力控制。
type P13UnionFightPolicy struct {
	ID                int64  `gorm:"primaryKey" json:"id"`
	EnrollmentEnabled bool   `gorm:"not null;default:false" json:"enrollmentEnabled"`
	MatchEnabled      bool   `gorm:"not null;default:false" json:"matchEnabled"`
	Source            string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightPolicy) TableName() string { return "p13_union_fight_policies" }

package db

// P4CopyBenefitRule 保存副本福利运营窗口，首次实际使用后冻结完整规则。
type P4CopyBenefitRule struct {
	ID             string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	Enabled        bool   `gorm:"not null" json:"enabled"`
	StartAt        int64  `gorm:"not null" json:"startAt"`
	EndAt          int64  `gorm:"not null" json:"endAt"`
	WeekdayMask    int32  `gorm:"not null" json:"weekdayMask"`
	StartMinute    int32  `gorm:"not null" json:"startMinute"`
	EndMinute      int32  `gorm:"not null" json:"endMinute"`
	CurrencyFactor int32  `gorm:"not null" json:"currencyFactor"`
	DropFactor     int32  `gorm:"not null" json:"dropFactor"`
	Frozen         []byte `gorm:"type:longblob" json:"-"`
	Source         string `gorm:"type:text;not null" json:"source"`
}

func (P4CopyBenefitRule) TableName() string { return "p4_copy_benefit_rules" }

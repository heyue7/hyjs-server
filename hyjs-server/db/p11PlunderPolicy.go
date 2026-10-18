package db

// P11PlunderPolicy 冻结原免战道具时间，未知攻击清除/上限规则默认关闭新使用。
type P11PlunderPolicy struct {
	ID            int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	AvoidEnabled  bool   `gorm:"not null" json:"avoid_enabled"`
	ItemHoursJSON string `gorm:"type:text;not null" json:"item_hours_json"`
	ItemHoursHash string `gorm:"type:varchar(64);not null" json:"item_hours_hash"`
	Source        string `gorm:"type:text;not null" json:"source"`
}

func (P11PlunderPolicy) TableName() string { return "p11_plunder_policies" }

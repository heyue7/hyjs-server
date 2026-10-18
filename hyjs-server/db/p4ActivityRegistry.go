package db

// P4ActivityRegistry 登记36种原活动类型；登记不等于规则或业务已恢复。
type P4ActivityRegistry struct {
	ID        int32  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Name      string `gorm:"type:varbinary(64);not null;uniqueIndex" json:"name"`
	Prefix    string `gorm:"type:varbinary(64);not null" json:"prefix"`
	Protocols string `gorm:"type:text;not null" json:"protocols"`
	Owner     string `gorm:"size:64;not null" json:"owner"`
	Status    string `gorm:"size:64;not null" json:"status"`
	Evidence  string `gorm:"type:text;not null" json:"evidence"`
}

func (P4ActivityRegistry) TableName() string { return "p4_activity_registry" }

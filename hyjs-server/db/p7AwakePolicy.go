package db

// P7AwakePolicy 保留原表规则来源和禁用的兼容开放等级。
type P7AwakePolicy struct {
	ID       int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled  bool   `gorm:"not null" json:"enabled"`
	MinLevel int32  `gorm:"not null" json:"minLevel"`
	Source   string `gorm:"type:text;not null" json:"source"`
}

func (P7AwakePolicy) TableName() string { return "p7_awake_policies" }

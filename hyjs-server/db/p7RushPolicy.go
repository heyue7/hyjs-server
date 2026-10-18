package db

// P7RushPolicy 将未知原运营费用显式登记，门禁关闭时禁止任何付费操作。
type P7RushPolicy struct {
	ID             int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled        bool   `gorm:"not null" json:"enabled"`
	EliteBuyCost   int32  `gorm:"not null" json:"eliteBuyCost"`
	TrialResetCost int32  `gorm:"not null" json:"trialResetCost"`
	Source         string `gorm:"type:text;not null" json:"source"`
}

func (P7RushPolicy) TableName() string { return "p7_rush_policies" }

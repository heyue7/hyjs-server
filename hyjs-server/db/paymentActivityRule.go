package db

// PaymentActivityRule 各条件独立窗口；运营未知档位明确注明本地兼容来源。
type PaymentActivityRule struct {
	ID          string `gorm:"primaryKey;size:128" json:"id"`
	Kind        string `gorm:"size:32;not null" json:"kind"`
	StartAt     int64  `gorm:"not null" json:"start_at"`
	EndAt       int64  `gorm:"not null" json:"end_at"`
	RewardEndAt int64  `gorm:"not null" json:"reward_end_at"`
	Threshold   int64  `gorm:"not null" json:"threshold"`
	Reward      string `gorm:"type:text;not null" json:"reward"`
	Name        string `gorm:"type:text;not null" json:"name"`
	Source      string `gorm:"type:text;not null" json:"source"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
}

func (PaymentActivityRule) TableName() string { return "p3_payment_activity_rules" }

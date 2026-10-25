package db

// FieldResourcePolicy 保存普通野外首领的全局资源规则，世界首领使用独立域。
type FieldResourcePolicy struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	BossDailyRewards   int64  `gorm:"not null;default:-1" json:"bossDailyRewards"`
	GatherRefreshHours string `gorm:"size:128;not null;default:'[0,12]'" json:"gatherRefreshHours"`
	Revision           int64  `gorm:"not null;default:0" json:"revision"`
}

func (FieldResourcePolicy) TableName() string { return "field_resource_policies" }

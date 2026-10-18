package db

// P13UnionFightItemPolicy 独立控制新道具消费，旧场次来源不能随热配置重抽。
type P13UnionFightItemPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null;default:false" json:"enabled"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightItemPolicy) TableName() string { return "p13_union_fight_item_policies" }

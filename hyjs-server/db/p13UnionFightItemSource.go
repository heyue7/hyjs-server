package db

// P13UnionFightItemSource 提前具名发布当前未留原数额的恢复比例；没有默认奖励或战斗winner。
type P13UnionFightItemSource struct {
	Day         string `gorm:"primaryKey;type:char(10)" json:"day"`
	ItemID      string `gorm:"primaryKey;type:varchar(64)" json:"itemId"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	HPPercent   int64  `gorm:"not null" json:"hpPercent"`
	PublishedAt int64  `gorm:"not null;autoCreateTime:false" json:"publishedAt"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightItemSource) TableName() string { return "p13_union_fight_item_sources" }

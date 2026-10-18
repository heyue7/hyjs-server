package db

// P13UnionFightCarItemSource 发布原小/大型炸弹未留下的伤害量，默认没有隐含伤害。
type P13UnionFightCarItemSource struct {
	Day         string `gorm:"primaryKey;type:char(10)" json:"day"`
	ItemID      string `gorm:"primaryKey;type:varchar(64)" json:"itemId"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	HPPercent   int64  `gorm:"not null" json:"hpPercent"`
	PublishedAt int64  `gorm:"not null;autoCreateTime:false" json:"publishedAt"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightCarItemSource) TableName() string { return "p13_union_fight_car_item_sources" }

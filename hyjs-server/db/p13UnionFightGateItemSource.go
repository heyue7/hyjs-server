package db

// P13UnionFightGateItemSource 提前发布原修复/攻城两个道具未留的数额，禁止从物品描述猜数字。
type P13UnionFightGateItemSource struct {
	Day         string `gorm:"primaryKey;type:char(10)" json:"day"`
	ItemID      string `gorm:"primaryKey;type:varchar(64)" json:"itemId"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	HPPercent   int64  `gorm:"not null" json:"hpPercent"`
	PublishedAt int64  `gorm:"not null;autoCreateTime:false" json:"publishedAt"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightGateItemSource) TableName() string { return "p13_union_fight_gate_item_sources" }

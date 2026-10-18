package db

// P13UnionFightItemOffer 冻结单场原目录与具名效果参数，关源后已冻结场次继续履约。
type P13UnionFightItemOffer struct {
	MatchID        string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	ItemID         string `gorm:"primaryKey;type:varchar(64)" json:"itemId"`
	HPPercent      int64  `gorm:"not null" json:"hpPercent"`
	ItemType       int64  `gorm:"not null" json:"itemType"`
	CooldownMillis int64  `gorm:"not null" json:"cooldownMillis"`
	Maximum        int64  `gorm:"not null" json:"maximum"`
	Source         string `gorm:"type:text;not null" json:"source"`
	SourceHash     string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightItemOffer) TableName() string { return "p13_union_fight_item_offers" }

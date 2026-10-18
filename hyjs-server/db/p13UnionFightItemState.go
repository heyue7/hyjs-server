package db

// P13UnionFightItemState 保存本人两类绝对到期时间，重登只投影真实剩余冷却。
type P13UnionFightItemState struct {
	MatchID      string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	NormalUntil  int64  `gorm:"not null;default:0" json:"normalUntil"`
	DiamondUntil int64  `gorm:"not null;default:0" json:"diamondUntil"`
	SourceHash   string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightItemState) TableName() string { return "p13_union_fight_item_states" }

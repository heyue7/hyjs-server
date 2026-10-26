package db

// P13UnionFightGateImpact retains each actual gate HP transition and its
// frozen car/item source independently of replay payload retention.
type P13UnionFightGateImpact struct {
	ID              string `gorm:"primaryKey;type:char(64)" json:"id"`
	MatchID         string `gorm:"type:char(64);not null;uniqueIndex:p13_gate_impact_order,priority:1;index:p13_gate_impact_time,priority:1" json:"matchId"`
	GateGUID        int64  `gorm:"not null;uniqueIndex:p13_gate_impact_order,priority:2" json:"gateGuid"`
	Ordinal         int64  `gorm:"not null;uniqueIndex:p13_gate_impact_order,priority:3" json:"ordinal"`
	Kind            string `gorm:"type:varchar(16);not null" json:"kind"`
	OfferHash       string `gorm:"type:char(64);not null" json:"offerHash"`
	PlayerID        int64  `gorm:"not null;default:0" json:"playerId"`
	Stamp           int64  `gorm:"not null;default:0" json:"stamp"`
	ItemID          string `gorm:"type:varchar(32);not null;default:''" json:"itemId"`
	InventoryBefore int32  `gorm:"not null;default:0" json:"inventoryBefore"`
	InventoryAfter  int32  `gorm:"not null;default:0" json:"inventoryAfter"`
	CarGUID         int64  `gorm:"not null;default:0" json:"carGuid"`
	HitOrdinal      int64  `gorm:"not null;default:0" json:"hitOrdinal"`
	Percent         int64  `gorm:"not null" json:"percent"`
	LogicalAt       int64  `gorm:"not null;index:p13_gate_impact_time,priority:2" json:"logicalAt"`
	AppliedAt       int64  `gorm:"not null" json:"appliedAt"`
	Before          []byte `gorm:"type:blob;not null" json:"-"`
	After           []byte `gorm:"type:blob;not null" json:"-"`
	BeforeHash      string `gorm:"type:char(64);not null" json:"beforeHash"`
	AfterHash       string `gorm:"type:char(64);not null" json:"afterHash"`
	SourceHash      string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightGateImpact) TableName() string { return "p13_union_fight_gate_impacts" }

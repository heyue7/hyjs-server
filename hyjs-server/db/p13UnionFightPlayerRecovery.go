package db

// P13UnionFightPlayerRecovery retains each actual item/aura life transition.
// It does not mix wallet mutations, combat results or unrelated event payloads.
type P13UnionFightPlayerRecovery struct {
	PendingID       string `gorm:"type:char(64);not null;default:'';index:p13_recovery_pending" json:"pendingId,omitempty"`
	ID              string `gorm:"primaryKey;type:char(64)" json:"id"`
	MatchID         string `gorm:"type:char(64);not null;index:p13_recovery_order,priority:1" json:"matchId"`
	PlayerID        int64  `gorm:"not null;index:p13_recovery_order,priority:2" json:"playerId"`
	Kind            string `gorm:"type:varchar(8);not null" json:"kind"`
	ItemID          string `gorm:"type:varchar(32);not null" json:"itemId"`
	Stamp           int64  `gorm:"not null;default:0" json:"stamp"`
	Nonce           string `gorm:"type:varchar(32);not null;default:''" json:"nonce,omitempty"`
	CasterID        int64  `gorm:"not null" json:"casterId"`
	EffectGUID      int64  `gorm:"not null;default:0" json:"effectGuid"`
	OfferHash       string `gorm:"type:char(64);not null" json:"offerHash"`
	HPPercent       int64  `gorm:"not null" json:"hpPercent"`
	InventoryBefore int32  `gorm:"not null;default:0" json:"inventoryBefore"`
	InventoryAfter  int32  `gorm:"not null;default:0" json:"inventoryAfter"`
	DeadOrdinal     int64  `gorm:"not null" json:"deadOrdinal"`
	LogicalAt       int64  `gorm:"not null;index:p13_recovery_order,priority:3" json:"logicalAt"`
	AppliedAt       int64  `gorm:"not null" json:"appliedAt"`
	Before          []byte `gorm:"type:blob;not null" json:"-"`
	After           []byte `gorm:"type:blob;not null" json:"-"`
	BeforeHash      string `gorm:"type:char(64);not null" json:"beforeHash"`
	AfterHash       string `gorm:"type:char(64);not null" json:"afterHash"`
	SourceHash      string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightPlayerRecovery) TableName() string { return "p13_union_fight_player_recoveries" }

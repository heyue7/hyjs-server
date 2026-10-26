package db

// P13UnionFightCarImpact preserves every successful bomb, including stamp zero.
// A real generation GUID owns its own immutable damage sequence.
type P13UnionFightCarImpact struct {
	ID                   string  `gorm:"primaryKey;type:char(64)" json:"id"`
	MatchID              string  `gorm:"type:char(64);not null;uniqueIndex:p13_car_impact_seq,priority:1;index:p13_car_impact_owner,priority:1" json:"matchId"`
	CarGUID              int64   `gorm:"not null;uniqueIndex:p13_car_impact_seq,priority:2" json:"carGUID"`
	Ordinal              int64   `gorm:"not null;uniqueIndex:p13_car_impact_seq,priority:3" json:"ordinal"`
	FactoryGUID          int64   `gorm:"not null" json:"factoryGUID"`
	FactoryID            int64   `gorm:"not null" json:"factoryID"`
	Generation           int64   `gorm:"not null" json:"generation"`
	BornAt               int64   `gorm:"not null" json:"bornAt"`
	CarCamp              int64   `gorm:"not null" json:"carCamp"`
	FactorySourceHash    string  `gorm:"type:char(64);not null" json:"factorySourceHash"`
	GenerationSourceHash string  `gorm:"type:char(64);not null" json:"generationSourceHash"`
	IdentityHash         string  `gorm:"type:char(64);not null" json:"identityHash"`
	PlayerID             int64   `gorm:"not null;index:p13_car_impact_owner,priority:2" json:"playerId"`
	Camp                 int64   `gorm:"not null" json:"camp"`
	ItemID               string  `gorm:"type:varchar(64);not null" json:"itemId"`
	Stamp                int64   `gorm:"not null;index:p13_car_impact_owner,priority:3" json:"stamp"`
	OfferHash            string  `gorm:"type:char(64);not null" json:"offerHash"`
	HPPercent            int64   `gorm:"not null" json:"hpPercent"`
	InventoryBefore      int64   `gorm:"not null" json:"inventoryBefore"`
	InventoryAfter       int64   `gorm:"not null" json:"inventoryAfter"`
	LogicalAt            int64   `gorm:"not null" json:"logicalAt"`
	AppliedAt            int64   `gorm:"not null" json:"appliedAt"`
	HPBefore             float64 `gorm:"not null" json:"hpBefore"`
	HPAfter              float64 `gorm:"not null" json:"hpAfter"`
	Before               []byte  `gorm:"type:blob;not null" json:"before"`
	After                []byte  `gorm:"type:blob;not null" json:"after"`
	BeforeHash           string  `gorm:"type:char(64);not null" json:"beforeHash"`
	AfterHash            string  `gorm:"type:char(64);not null" json:"afterHash"`
	SourceHash           string  `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightCarImpact) TableName() string { return "p13_union_fight_car_impacts" }

package db

// P1GatherCatalogGuard serializes changes to the set of periods for one
// gather fact kind. Rows are initialized before the P1 runtime is enabled.
type P1GatherCatalogGuard struct {
	Kind string `gorm:"type:varbinary(64);primaryKey" json:"kind"`
}

func (P1GatherCatalogGuard) TableName() string { return "p1_gather_catalog_guards" }

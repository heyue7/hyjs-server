package db

// P7ExpeditionCataloguePolicy enables a named local earned-grade publication policy.
type P7ExpeditionCataloguePolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P7ExpeditionCataloguePolicy) TableName() string { return "p7_expedition_catalogue_policies" }

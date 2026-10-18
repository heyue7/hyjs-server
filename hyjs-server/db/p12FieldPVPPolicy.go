package db

// P12FieldPVPPolicy publishes the finite local rule before enabling any transaction.
type P12FieldPVPPolicy struct {
	ID         int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled    bool   `gorm:"not null;default:false" json:"enabled"`
	PolicyName string `gorm:"size:96;not null" json:"policyName"`
	Source     string `gorm:"type:longtext;not null" json:"source"`
}

func (P12FieldPVPPolicy) TableName() string { return "p12_field_pvp_policies" }

package db

// P14FinalResultPolicy is separately published for canonical result federation.
type P14FinalResultPolicy struct {
	ScopeProof  string `gorm:"type:char(64);not null" json:"scopeProof"`
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
}

func (P14FinalResultPolicy) TableName() string { return "p14_final_result_policies" }

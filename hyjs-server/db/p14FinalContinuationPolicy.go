package db

// P14FinalContinuationPolicy enables only the explicitly controlled local wrapper.
type P14FinalContinuationPolicy struct {
	ScopeProof          string `gorm:"type:char(64);not null" json:"scopeProof"`
	ID                  int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled             bool   `gorm:"not null" json:"enabled"`
	Version             string `gorm:"type:varchar(128);not null" json:"version"`
	Source              string `gorm:"type:varchar(1024);not null" json:"source"`
	PublishedAt         int64  `gorm:"not null" json:"publishedAt"`
	WrapperRule         string `gorm:"type:varchar(128);not null" json:"wrapperRule"`
	WrapperManifestHash string `gorm:"type:char(64);not null" json:"wrapperManifestHash"`
	WrapperDLLHash      string `gorm:"type:char(64);not null" json:"wrapperDllHash"`
}

func (P14FinalContinuationPolicy) TableName() string { return "p14_final_continuation_policies" }

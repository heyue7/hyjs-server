package db

// P14AuditionQualification 只有真实权威比赛产生最后十名存活者时才冻结晋级来源。
type P14AuditionQualification struct {
	// Optional fields keep legacy zero-value JSON digests byte compatible.
	RecoverySource      string `gorm:"type:varchar(1024);not null;default:''" json:"recovery_source,omitempty"`
	RecoveryPublishedAt int64  `gorm:"not null;default:0" json:"recovery_published_at,omitempty"`
	SeasonIndex         int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID            int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	BoundaryHash        string `gorm:"size:64;not null" json:"boundary_hash"`
	RoundCount          int64  `gorm:"not null" json:"round_count"`
	Players             []byte `gorm:"type:longblob;not null" json:"players"`
	ResultsHash         string `gorm:"size:64;not null" json:"results_hash"`
	QualifiedAt         int64  `gorm:"not null" json:"qualified_at"`
	SourceHash          string `gorm:"size:64;not null" json:"source_hash"`
}

func (P14AuditionQualification) TableName() string { return "p14_audition_qualifications" }

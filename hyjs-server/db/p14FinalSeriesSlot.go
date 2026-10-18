package db

// P14FinalSeriesSlot binds real series scheduling to its frozen source, without presumed battle outcomes.
type P14FinalSeriesSlot struct {
	SeriesID        string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	GameIndex       int32  `gorm:"primaryKey;autoIncrement:false" json:"gameIndex"`
	ScheduledAt     int64  `gorm:"not null" json:"scheduledAt"`
	LockAt          int64  `gorm:"not null" json:"lockAt"`
	State           int32  `gorm:"not null" json:"state"`
	InputID         string `gorm:"type:varchar(64);not null" json:"inputId"`
	Input           []byte `gorm:"type:longblob;not null" json:"input"`
	InputHash       string `gorm:"type:varchar(64);not null" json:"inputHash"`
	ProvenanceHash  string `gorm:"type:varchar(64);not null" json:"provenanceHash"`
	Certificate     []byte `gorm:"type:longblob;not null" json:"certificate"`
	CertificateHash string `gorm:"type:varchar(64);not null" json:"certificateHash"`
}

func (P14FinalSeriesSlot) TableName() string { return "p14_final_series_slots" }

package db

// P14FinalContinuationInput freezes this slot's actual ordered source and a new seed.
// It does not carry HP/AP from another game or synthesize a played result.
type P14FinalContinuationInput struct {
	SeriesID           string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	GameIndex          int32  `gorm:"primaryKey;autoIncrement:false" json:"gameIndex"`
	ID                 string `gorm:"type:char(64);not null;uniqueIndex" json:"id"`
	OfferHash          string `gorm:"type:char(64);not null" json:"offerHash"`
	PreviousResultHash string `gorm:"type:char(64);not null" json:"previousResultHash"`
	Replay             []byte `gorm:"type:longblob;not null" json:"-"`
	ReplayHash         string `gorm:"type:char(64);not null" json:"replayHash"`
	Provenance         []byte `gorm:"type:longblob;not null" json:"-"`
	ProvenanceHash     string `gorm:"type:char(64);not null" json:"provenanceHash"`
}

func (P14FinalContinuationInput) TableName() string { return "p14_final_continuation_inputs" }

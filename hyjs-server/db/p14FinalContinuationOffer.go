package db

// P14FinalContinuationOffer preserves an independently published operational rule.
type P14FinalContinuationOffer struct {
	SeriesID           string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	SeriesManifestHash string `gorm:"type:char(64);not null" json:"seriesManifestHash"`
	Policy             []byte `gorm:"type:longblob;not null" json:"policy"`
	Rules              string `gorm:"type:varchar(1024);not null" json:"rules"`
	SourceHash         string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14FinalContinuationOffer) TableName() string { return "p14_final_continuation_offers" }

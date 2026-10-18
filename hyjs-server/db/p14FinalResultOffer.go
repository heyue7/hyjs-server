package db

// P14FinalResultOffer preserves the independent prepublished forwarding rule.
type P14FinalResultOffer struct {
	SeriesID           string `gorm:"type:char(64);primaryKey" json:"seriesId"`
	SeriesManifestHash string `gorm:"type:char(64);not null" json:"seriesManifestHash"`
	Policy             []byte `gorm:"type:longblob;not null" json:"policy"`
	Rules              string `gorm:"type:varchar(1024);not null" json:"rules"`
	SourceHash         string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P14FinalResultOffer) TableName() string { return "p14_final_result_offers" }

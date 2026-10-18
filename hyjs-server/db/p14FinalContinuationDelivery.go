package db

// P14FinalContinuationDelivery retains exact origin HMAC payloads until acknowledged.
type P14FinalContinuationDelivery struct {
	SeriesID            string `gorm:"type:char(64);not null;uniqueIndex:idx_p14_final_continuation_destination" json:"seriesId"`
	GameIndex           int32  `gorm:"not null;uniqueIndex:idx_p14_final_continuation_destination" json:"gameIndex"`
	SourceResultHash    string `gorm:"type:char(64);not null" json:"sourceResultHash"`
	ID                  string `gorm:"type:char(64);primaryKey" json:"id"`
	DestinationServerID int64  `gorm:"not null;index;uniqueIndex:idx_p14_final_continuation_destination" json:"destinationServerId"`
	Payload             []byte `gorm:"type:longblob;not null" json:"-"`
	PayloadHash         string `gorm:"type:char(64);not null" json:"payloadHash"`
	Confirmed           bool   `gorm:"not null;index" json:"confirmed"`
	AttemptCount        int64  `gorm:"not null" json:"attemptCount"`
	CreatedAt           int64  `gorm:"not null" json:"createdAt"`
	LastAttemptAt       int64  `gorm:"not null" json:"lastAttemptAt"`
}

func (P14FinalContinuationDelivery) TableName() string { return "p14_final_continuation_deliveries" }

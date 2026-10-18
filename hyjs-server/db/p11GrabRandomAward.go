package db

// P11GrabRandomAward consumes one actual-time ordinal and stores its crypto draw.
type P11GrabRandomAward struct {
	Month          string `gorm:"type:varchar(6);primaryKey" json:"month"`
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false;uniqueIndex:uq_p11_grab_random_stamp,priority:1" json:"playerId"`
	Ordinal        int64  `gorm:"primaryKey;autoIncrement:false" json:"ordinal"`
	Stamp          int64  `gorm:"uniqueIndex:uq_p11_grab_random_stamp,priority:2;not null" json:"stamp"`
	Day            string `gorm:"type:varchar(8);not null" json:"day"`
	EligibleMillis int64  `gorm:"not null" json:"eligibleMillis"`
	ResourceID     string `gorm:"type:varchar(64);not null" json:"resourceId"`
	Quantity       int32  `gorm:"not null" json:"quantity"`
	Draw           int64  `gorm:"not null" json:"draw"`
	DrawTotal      int64  `gorm:"not null" json:"drawTotal"`
	SourceHash     string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	Response       []byte `gorm:"type:longblob;not null" json:"response"`
	IntegrityHash  string `gorm:"type:varchar(64);not null" json:"integrityHash"`
	CreatedAt      int64  `gorm:"not null" json:"createdAt"`
}

func (P11GrabRandomAward) TableName() string { return "p11_grab_random_awards" }

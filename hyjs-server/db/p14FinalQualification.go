package db

// P14FinalQualification freezes exactly one complete federation result, without filling empty places.
type P14FinalQualification struct {
	SeasonIndex int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	Content     []byte `gorm:"type:longblob;not null" json:"-"`
	ContentHash string `gorm:"type:char(64);not null" json:"content_hash"`
	ObservedAt  int64  `gorm:"not null" json:"observed_at"`
}

func (P14FinalQualification) TableName() string { return "p14_final_qualifications" }

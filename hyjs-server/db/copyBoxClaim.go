package db

// CopyBoxClaim freezes the original chest when first actually entering its mission.
type CopyBoxClaim struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	BoxID     string `gorm:"type:varbinary(64);primaryKey" json:"box_id"`
	Input     []byte `gorm:"type:longblob;not null" json:"input"`
	InputHash string `gorm:"type:char(64);not null" json:"input_hash"`
	Claimed   bool   `gorm:"not null" json:"claimed"`
}

func (CopyBoxClaim) TableName() string { return "copy_box_claims" }

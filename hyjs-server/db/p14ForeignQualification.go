package db

// P14ForeignQualification 按赛季和可信原服保存一份不可覆写的截止证书，内部ID包含目的服。
type P14ForeignQualification struct {
	SeasonIndex       int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	DeliveryID        string `gorm:"type:char(64);not null;uniqueIndex" json:"delivery_id"`
	Payload           []byte `gorm:"type:longblob;not null" json:"payload"`
	PayloadHash       string `gorm:"type:char(64);not null" json:"payload_hash"`
	BoundaryHash      string `gorm:"type:char(64);not null" json:"boundary_hash"`
	QualificationHash string `gorm:"type:char(64);not null" json:"qualification_hash"`
	ImportedAt        int64  `gorm:"not null" json:"imported_at"`
}

func (P14ForeignQualification) TableName() string { return "p14_foreign_qualifications" }

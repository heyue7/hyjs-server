package db

// GGOFieldLine serializes the bounded public scene snapshot for one realm line.
type GGOFieldLine struct {
	ServerID  int64  `gorm:"primaryKey;autoIncrement:false"`
	Line      int64  `gorm:"primaryKey;autoIncrement:false"`
	Revision  int64  `gorm:"not null"`
	Payload   []byte `gorm:"type:longblob;not null"`
	UpdatedAt int64  `gorm:"not null;autoUpdateTime:false"`
}

func (GGOFieldLine) TableName() string { return "ggo_field_lines" }

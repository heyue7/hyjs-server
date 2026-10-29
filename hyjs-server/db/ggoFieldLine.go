package db

// GGOFieldLine serializes the bounded public scene snapshot for one realm line.
type GGOFieldLine struct {
	ServerID  int64  `gorm:"primaryKey;autoIncrement:false;index:idx_ggo_lines_due,priority:1"`
	Line      int64  `gorm:"primaryKey;autoIncrement:false"`
	Revision  int64  `gorm:"not null"`
	Payload   []byte `gorm:"type:longblob;not null"`
	UpdatedAt int64  `gorm:"not null;autoUpdateTime:false"`
	NextDueAt int64  `gorm:"not null;default:0;index:idx_ggo_lines_due,priority:2"`
}

func (GGOFieldLine) TableName() string { return "ggo_field_lines" }

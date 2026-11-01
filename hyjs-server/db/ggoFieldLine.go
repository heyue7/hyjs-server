package db

// GGOFieldLine serializes the bounded public scene snapshot for one realm line.
type GGOFieldLine struct {
	ServerID           int64  `gorm:"primaryKey;autoIncrement:false;index:idx_ggo_lines_due,priority:1"`
	Line               int64  `gorm:"primaryKey;autoIncrement:false"`
	Revision           int64  `gorm:"not null"`
	Payload            []byte `gorm:"type:longblob;not null"`
	UpdatedAt          int64  `gorm:"not null;autoUpdateTime:false"`
	NextDueAt          int64  `gorm:"not null;default:0;index:idx_ggo_lines_due,priority:2"`
	PointStateReady    bool   `gorm:"not null;default:false" json:"pointStateReady"`
	DeploymentRevision string `gorm:"size:128;not null;default:''" json:"deploymentRevision"`
	CycleMS            int64  `gorm:"not null;default:0" json:"cycleMs"`
	GenerationFloor    int64  `gorm:"not null;default:0" json:"generationFloor"`
}

func (GGOFieldLine) TableName() string { return "ggo_field_lines" }

package db

// GGOFieldPoint stores the runtime state of one point on a realm line.
type GGOFieldPoint struct {
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false;index:idx_ggo_point_monster,priority:1;index:idx_ggo_point_due,priority:1" json:"serverId"`
	Line        int64  `gorm:"primaryKey;autoIncrement:false;index:idx_ggo_point_monster,priority:2;index:idx_ggo_point_due,priority:3" json:"line"`
	PointID     string `gorm:"primaryKey;type:varbinary(64);size:64;index:idx_ggo_point_due,priority:4" json:"pointId"`
	Revision    int64  `gorm:"not null" json:"revision"`
	Payload     []byte `gorm:"type:longblob;not null" json:"-"`
	MonsterUUID string `gorm:"size:64;not null;default:'';index:idx_ggo_point_monster,priority:3" json:"monsterUuid"`
	NextDueAt   int64  `gorm:"not null;index:idx_ggo_point_due,priority:2" json:"nextDueAt"`
}

func (GGOFieldPoint) TableName() string { return "ggo_field_points" }

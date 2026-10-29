package db

// SharedFieldBoss55 owns one ordinary boss generation per realm, scene, line and position.
type SharedFieldBoss55 struct {
	DeathRules73 []byte `gorm:"type:longblob"`

	ServerID  int64  `gorm:"primaryKey;autoIncrement:false"`
	Scene     string `gorm:"primaryKey;type:varbinary(64)"`
	Line      int64  `gorm:"primaryKey;autoIncrement:false;not null;default:1"`
	PosID     string `gorm:"primaryKey;type:varbinary(64)"`
	UUID      string `gorm:"type:varbinary(128);not null"`
	Payload   []byte `gorm:"type:longblob;not null"`
	HP        []byte `gorm:"type:longblob;not null"`
	RespawnAt int64  `gorm:"not null"`
	KillerID  int64  `gorm:"not null"`
}

func (SharedFieldBoss55) TableName() string { return "shared_field_boss55" }

package db

// P7ExpeditionHero 独占一名真实伙伴，snapshot供保存钩子核对被占用实例未被修改/回收。
type P7ExpeditionHero struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	GUID          string `gorm:"primaryKey;type:varbinary(128)" json:"guid"`
	MissionSerial string `gorm:"type:varbinary(64);not null;index" json:"missionSerial"`
	Snapshot      []byte `gorm:"type:longblob;not null" json:"-"`
}

func (P7ExpeditionHero) TableName() string { return "p7_expedition_heroes" }

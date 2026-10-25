package db

// P8ResourceNode 保存单人地图真实资源点、剩余次数及重生时刻。
type P8ResourceNode struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Scene       string `gorm:"primaryKey;size:64" json:"scene"`
	PosID       string `gorm:"primaryKey;size:64" json:"posId"`
	ResourceID  string `gorm:"size:64;not null" json:"resourceId"`
	Remaining   int32  `gorm:"not null" json:"remaining"`
	RefreshAt   int64  `gorm:"not null" json:"refreshAt"`
	ScheduledAt int64  `gorm:"not null;default:0" json:"scheduledAt"`
	Generation  int64  `gorm:"not null" json:"generation"`
	Snapshot    string `gorm:"type:longtext;not null" json:"-"`
}

func (P8ResourceNode) TableName() string { return "p8_resource_nodes" }

package db

// P13UnionFightResourceNode 保存本场冻结矿点、原3秒/5次、真实未耗次数与唯一采集租约。
type P13UnionFightResourceNode struct {
	MatchID       string  `gorm:"primaryKey;type:char(64)" json:"matchId"`
	GUID          int64   `gorm:"primaryKey;autoIncrement:false" json:"guid"`
	Frame         []byte  `gorm:"type:blob;not null" json:"-"`
	SourceHash    string  `gorm:"type:char(64);not null" json:"sourceHash"`
	FrameHash     string  `gorm:"type:char(64);not null" json:"frameHash"`
	X             float32 `gorm:"not null" json:"x"`
	Y             float32 `gorm:"not null" json:"y"`
	Z             float32 `gorm:"not null" json:"z"`
	TotalUses     int64   `gorm:"not null" json:"totalUses"`
	RemainingUses int64   `gorm:"not null" json:"remainingUses"`
	GatherMillis  int64   `gorm:"not null" json:"gatherMillis"`
	BusyTaskID    string  `gorm:"type:char(64);not null;default:''" json:"busyTaskId"`
	Source        string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightResourceNode) TableName() string { return "p13_union_fight_resource_nodes" }

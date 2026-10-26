package db

// P13UnionFightGateState 保存实际八门动态HP，原实例身份/初始快照与每次原帧SHA共同校验。
type P13UnionFightGateState struct {
	ImpactOrdinal int64  `gorm:"not null;default:0" json:"impactOrdinal,omitempty"`
	MatchID       string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	GUID          int64  `gorm:"primaryKey;autoIncrement:false" json:"guid"`
	GateID        int64  `gorm:"not null" json:"gateId"`
	Camp          int64  `gorm:"not null" json:"camp"`
	Frame         []byte `gorm:"type:longblob;not null" json:"-"`
	FrameHash     string `gorm:"type:char(64);not null" json:"frameHash"`
	InitialHash   string `gorm:"type:char(64);not null" json:"initialHash"`
	Source        string `gorm:"type:text;not null" json:"source"`
	SourceHash    string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightGateState) TableName() string { return "p13_union_fight_gate_states" }

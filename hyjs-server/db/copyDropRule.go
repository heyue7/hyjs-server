package db

// CopyDropRule 记录缺失原服抽样实现的具名本地兼容策略，不修改原掉落表。
type CopyDropRule struct {
	ID      string `gorm:"column:id;primaryKey;size:32" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Mode    string `gorm:"size:64;not null" json:"mode"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (CopyDropRule) TableName() string { return "copy_drop_rules" }

// CopyDropSnapshot 随开始战斗冻结原数量/权重与当时策略，热改不重写已开始实例。
type CopyDropSnapshot struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	FightID   string `gorm:"primaryKey;size:64" json:"fightId"`
	TaskID    string `gorm:"size:32;not null" json:"taskId"`
	Input     []byte `gorm:"type:longblob;not null" json:"input"`
	InputHash string `gorm:"size:64;not null" json:"inputHash"`
}

func (CopyDropSnapshot) TableName() string { return "copy_drop_snapshots" }

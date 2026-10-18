package db

// P13UnionFightRevivePointSource 提前具名公布原中立点的当地人数/争夺与采样规则，默认关闭。
type P13UnionFightRevivePointSource struct {
	Day           string `gorm:"primaryKey;type:char(10)" json:"day"`
	PointID       int64  `gorm:"primaryKey;autoIncrement:false" json:"point_id"`
	Enabled       bool   `gorm:"not null;default:false" json:"enabled"`
	MaxGapMillis  int64  `gorm:"not null" json:"max_gap_millis"`
	MaxStepMillis int64  `gorm:"not null" json:"max_step_millis"`
	PublishedAt   int64  `gorm:"not null" json:"published_at"`
	Source        string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightRevivePointSource) TableName() string {
	return "p13_union_fight_revive_point_sources"
}

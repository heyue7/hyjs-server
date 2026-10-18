package db

// P13UnionFightResourceSource 记录显式发布的本地矿点来源；缺原spawn位置时无默认矿点或自动再生。
type P13UnionFightResourceSource struct {
	Day         string  `gorm:"primaryKey;type:char(10)" json:"day"`
	SourceKey   string  `gorm:"primaryKey;type:varchar(64)" json:"sourceKey"`
	Enabled     bool    `gorm:"not null;default:false" json:"enabled"`
	ResourceID  string  `gorm:"type:varchar(64);not null" json:"resourceId"`
	X           float32 `gorm:"not null" json:"x"`
	Y           float32 `gorm:"not null" json:"y"`
	Z           float32 `gorm:"not null" json:"z"`
	RewardRes   int64   `gorm:"not null" json:"rewardRes"`
	PublishedAt int64   `gorm:"not null" json:"publishedAt"`
	Source      string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightResourceSource) TableName() string { return "p13_union_fight_resource_sources" }

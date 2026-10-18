package db

// P13UnionFightCarMotionOffer 保存同场完整原路线与已明确发布的本地速度，热改来源不替换冻结值。
type P13UnionFightCarMotionOffer struct {
	MatchID     string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	FactoryGUID int64   `gorm:"primaryKey;autoIncrement:false" json:"factory_guid"`
	FactoryID   int64   `gorm:"not null" json:"factory_id"`
	Speed       float64 `gorm:"not null" json:"speed"`
	Path        []byte  `gorm:"type:longblob;not null" json:"path"`
	Source      string  `gorm:"type:text;not null" json:"source"`
	SourceHash  string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightCarMotionOffer) TableName() string { return "p13_union_fight_car_motion_offers" }

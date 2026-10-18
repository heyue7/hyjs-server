package db

// P13UnionFightRevivePointOffer 冻结原点GUID、水平半径及原速度，原争夺算法未恢复时明示本地规则。
type P13UnionFightRevivePointOffer struct {
	MatchID       string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	PointGUID     int64   `gorm:"primaryKey;autoIncrement:false" json:"point_guid"`
	PointID       int64   `gorm:"not null" json:"point_id"`
	X             float32 `gorm:"not null" json:"x"`
	Z             float32 `gorm:"not null" json:"z"`
	Radius        float64 `gorm:"not null" json:"radius"`
	Speed         float64 `gorm:"not null" json:"speed"`
	MaxGapMillis  int64   `gorm:"not null" json:"max_gap_millis"`
	MaxStepMillis int64   `gorm:"not null" json:"max_step_millis"`
	Source        string  `gorm:"type:text;not null" json:"source"`
	SourceHash    string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightRevivePointOffer) TableName() string { return "p13_union_fight_revive_point_offers" }

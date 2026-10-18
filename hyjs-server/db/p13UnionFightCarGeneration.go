package db

// P13UnionFightCarGeneration 每代独立战车保存原厂身份/实际出车时刻和冻结SHA，不覆盖既有车帧。
type P13UnionFightCarGeneration struct {
	MatchID         string `gorm:"primaryKey;uniqueIndex:p13_car_generation,priority:1;type:varchar(64)" json:"match_id"`
	CarGUID         int64  `gorm:"primaryKey;autoIncrement:false" json:"car_guid"`
	FactoryGUID     int64  `gorm:"not null;uniqueIndex:p13_car_generation,priority:2" json:"factory_guid"`
	Generation      int64  `gorm:"not null;uniqueIndex:p13_car_generation,priority:3" json:"generation"`
	BornAt          int64  `gorm:"not null" json:"born_at"`
	FactorySnapshot []byte `gorm:"not null;type:longblob" json:"factory_snapshot"`
	OfferHash       string `gorm:"not null;type:char(64)" json:"offer_hash"`
	SourceHash      string `gorm:"not null;type:char(64)" json:"source_hash"`
	CarFrame        []byte `gorm:"not null;type:longblob" json:"car_frame"`
	CarHash         string `gorm:"not null;type:char(64)" json:"car_hash"`
	StateHash       string `gorm:"not null;type:char(64)" json:"state_hash"`
}

func (P13UnionFightCarGeneration) TableName() string { return "p13_union_fight_car_generations" }

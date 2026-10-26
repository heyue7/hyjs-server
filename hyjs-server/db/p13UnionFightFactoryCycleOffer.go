package db

// P13UnionFightFactoryCycleOffer 冻结原厂120秒周期/动画及具名本地上限，与后续热配置隔离。
type P13UnionFightFactoryCycleOffer struct {
	MatchID         string `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	FactoryGUID     int64  `gorm:"primaryKey;autoIncrement:false" json:"factory_guid"`
	CraftMillis     int64  `gorm:"not null" json:"craft_millis"`
	AnimationMillis int64  `gorm:"not null" json:"animation_millis"`
	MaxGenerations  int64  `gorm:"not null" json:"max_generations"`
	MaxAlive        int64  `gorm:"not null" json:"max_alive"`
	Source          string `gorm:"not null;type:text" json:"source"`
	SourceHash      string `gorm:"not null;type:char(64)" json:"source_hash"`
	Rule            string `gorm:"type:varchar(64);not null;default:''" json:"rule,omitempty"`
	BufferCapacity  int64  `gorm:"not null;default:0" json:"buffer_capacity,omitempty"`
}

func (P13UnionFightFactoryCycleOffer) TableName() string {
	return "p13_union_fight_factory_cycle_offers"
}

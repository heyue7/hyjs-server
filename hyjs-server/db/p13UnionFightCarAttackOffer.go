package db

// P13UnionFightCarAttackOffer 保存原战车实际攻门的提前具名本地配置及持久计时。
type P13UnionFightCarAttackOffer struct {
	MatchID        string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	FactoryGUID    int64   `gorm:"primaryKey;autoIncrement:false" json:"factory_guid"`
	GateGUID       int64   `gorm:"not null" json:"gate_guid"`
	GateID         int64   `gorm:"not null" json:"gate_id"`
	HPPercent      int64   `gorm:"not null" json:"hp_percent"`
	Radius         float64 `gorm:"not null" json:"radius"`
	StopDistance   float64 `gorm:"not null" json:"stop_distance"`
	CooldownMillis int64   `gorm:"not null" json:"cooldown_millis"`
	ReleaseMillis  int64   `gorm:"not null" json:"release_millis"`
	MotionHash     string  `gorm:"type:char(64);not null" json:"motion_hash"`
	Source         string  `gorm:"type:text;not null" json:"source"`
	SourceHash     string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightCarAttackOffer) TableName() string { return "p13_union_fight_car_attack_offers" }

package db

// P13UnionFightCarAttackState 保存原战车实际攻门的提前具名本地配置及持久计时。
type P13UnionFightCarAttackState struct {
	MatchID          string `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	FactoryGUID      int64  `gorm:"primaryKey;autoIncrement:false" json:"factory_guid"`
	CarGUID          int64  `gorm:"not null" json:"car_guid"`
	NextAttackAt     int64  `gorm:"not null" json:"next_attack_at"`
	PendingReleaseAt int64  `gorm:"not null" json:"pending_release_at"`
	Completed        bool   `gorm:"not null;default:false" json:"completed"`
	Hits             int64  `gorm:"not null;default:0" json:"hits"`
	OfferHash        string `gorm:"type:char(64);not null" json:"offer_hash"`
	StateHash        string `gorm:"type:char(64);not null" json:"state_hash"`
}

func (P13UnionFightCarAttackState) TableName() string { return "p13_union_fight_car_attack_states" }

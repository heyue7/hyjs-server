package db

// P13UnionFightCarMotionState 持久化实际首车沿冻结路线的距离和时钟；到达终点不推断攻门或战争结果。
type P13UnionFightCarMotionState struct {
	MatchID     string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	FactoryGUID int64   `gorm:"primaryKey;autoIncrement:false" json:"factory_guid"`
	CarGUID     int64   `gorm:"not null" json:"car_guid"`
	Distance    float64 `gorm:"not null" json:"distance"`
	LastAt      int64   `gorm:"not null" json:"last_at"`
	Arrived     bool    `gorm:"not null;default:false" json:"arrived"`
	OfferHash   string  `gorm:"type:char(64);not null" json:"offer_hash"`
	StateHash   string  `gorm:"type:char(64);not null" json:"state_hash"`
}

func (P13UnionFightCarMotionState) TableName() string { return "p13_union_fight_car_motion_states" }

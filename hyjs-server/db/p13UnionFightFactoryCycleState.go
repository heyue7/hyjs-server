package db

// P13UnionFightFactoryCycleState 记录下一代真实厂动画与出车窗口，独立于首车事实和独立CarGUID。
type P13UnionFightFactoryCycleState struct {
	MatchID             string `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	FactoryGUID         int64  `gorm:"primaryKey;autoIncrement:false" json:"factory_guid"`
	NextBornAt          int64  `gorm:"not null" json:"next_born_at"`
	Generation          int64  `gorm:"not null" json:"generation"`
	AnimationPlayed     bool   `gorm:"not null;default:false" json:"animation_played"`
	ClosedAt            int64  `gorm:"not null;default:0" json:"closed_at"`
	OfferHash           string `gorm:"not null;type:char(64)" json:"offer_hash"`
	StateHash           string `gorm:"not null;type:char(64)" json:"state_hash"`
	PausedAt            int64  `gorm:"not null;default:0" json:"paused_at,omitempty"`
	RemainingMillis     int64  `gorm:"not null;default:0" json:"remaining_millis,omitempty"`
	ProductionStartedAt int64  `gorm:"not null;default:0" json:"production_started_at,omitempty"`
	BufferedAt          int64  `gorm:"not null;default:0" json:"buffered_at,omitempty"`
}

func (P13UnionFightFactoryCycleState) TableName() string {
	return "p13_union_fight_factory_cycle_states"
}

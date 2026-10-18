package db

// P14PromotionGrant 逐真实应发12点时刻记录封顶前后账目，补观察也不累计突破20点。
type P14PromotionGrant struct {
	SeasonIndex       int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	GrantAt           int64  `gorm:"primaryKey;autoIncrement:false" json:"grant_at"`
	ObservedAt        int64  `gorm:"not null" json:"observed_at"`
	PointsBefore      int64  `gorm:"not null" json:"points_before"`
	PointsAfter       int64  `gorm:"not null" json:"points_after"`
	RulesHash         string `gorm:"type:char(64);not null" json:"rules_hash"`
	QualificationHash string `gorm:"type:char(64);not null" json:"qualification_hash"`
	StateBeforeHash   string `gorm:"type:char(64);not null" json:"state_before_hash"`
	StateAfterHash    string `gorm:"type:char(64);not null" json:"state_after_hash"`
	SourceHash        string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P14PromotionGrant) TableName() string { return "p14_promotion_grants" }

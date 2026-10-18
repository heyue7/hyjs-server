package db

// P13UnionFightSpeedItemEffect 保存原015友方3米/5秒冲锋buff及提前具名倍率和未知资格的本地策略。
type P13UnionFightSpeedItemEffect struct {
	MatchID    string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	GUID       int64   `gorm:"primaryKey;autoIncrement:false" json:"guid"`
	TargetID   int64   `gorm:"not null;index:p13_speed_target,priority:2" json:"target_id"`
	CasterID   int64   `gorm:"not null" json:"caster_id"`
	StartedAt  int64   `gorm:"not null" json:"started_at"`
	ExpiresAt  int64   `gorm:"not null;index:p13_speed_pending,priority:2" json:"expires_at"`
	EndedAt    int64   `gorm:"not null;default:0;index:p13_speed_pending,priority:1" json:"ended_at"`
	Multiplier float64 `gorm:"not null" json:"multiplier"`
	OfferHash  string  `gorm:"type:char(64);not null" json:"offer_hash"`
	SourceHash string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightSpeedItemEffect) TableName() string { return "p13_union_fight_speed_item_effects" }

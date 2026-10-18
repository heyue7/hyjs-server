package db

// P13UnionFightHealAuraEffect 保存原013回复法阵的原buff009五秒及提前发布具名本地团队治疗策略。
type P13UnionFightHealAuraEffect struct {
	MatchID    string `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	GUID       int64  `gorm:"primaryKey;autoIncrement:false" json:"guid"`
	CasterID   int64  `gorm:"not null;index:p13_heal_caster" json:"caster_id"`
	StartedAt  int64  `gorm:"not null" json:"started_at"`
	ExpiresAt  int64  `gorm:"not null" json:"expires_at"`
	NextTickAt int64  `gorm:"not null;index:p13_heal_pending,priority:2" json:"next_tick_at"`
	EndedAt    int64  `gorm:"not null;default:0;index:p13_heal_pending,priority:1" json:"ended_at"`
	OfferHash  string `gorm:"type:char(64);not null" json:"offer_hash"`
	SourceHash string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightHealAuraEffect) TableName() string { return "p13_union_fight_heal_aura_effects" }

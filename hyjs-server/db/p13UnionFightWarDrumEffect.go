package db

// P13UnionFightWarDrumEffect 保存原014/totem1与提前明确发布的本地模型、范围、持续及攻击属性加成。
type P13UnionFightWarDrumEffect struct {
	MatchID    string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	GUID       int64   `gorm:"primaryKey;autoIncrement:false" json:"guid"`
	CasterID   int64   `gorm:"not null;index:p13_drum_caster" json:"caster_id"`
	GuildID    string  `gorm:"type:varchar(64);not null" json:"guild_id"`
	Camp       int64   `gorm:"not null" json:"camp"`
	X          float32 `gorm:"type:double;not null" json:"x"`
	Y          float32 `gorm:"type:double;not null" json:"y"`
	Z          float32 `gorm:"type:double;not null" json:"z"`
	StartedAt  int64   `gorm:"not null" json:"started_at"`
	ExpiresAt  int64   `gorm:"not null;index:p13_drum_pending,priority:2" json:"expires_at"`
	EndedAt    int64   `gorm:"not null;default:0;index:p13_drum_pending,priority:1" json:"ended_at"`
	OfferHash  string  `gorm:"type:char(64);not null" json:"offer_hash"`
	SourceHash string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightWarDrumEffect) TableName() string { return "p13_union_fight_war_drum_effects" }

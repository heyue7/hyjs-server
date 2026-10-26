package db

// P13UnionFightGuardState preserves the independent guard lifecycle; existing audit records remain untouched.
type P13UnionFightGuardState struct {
	GuardRule        string `gorm:"type:varchar(64);not null;default:''" json:"guard_rule,omitempty"`
	MatchID          string `gorm:"primaryKey;type:char(64)" json:"match_id"`
	GuardGUID        int64  `gorm:"primaryKey;autoIncrement:false" json:"guard_guid"`
	GuardID          string `gorm:"type:varchar(64);not null" json:"guard_id"`
	HeroID           string `gorm:"type:varchar(64);not null" json:"hero_id"`
	Camp             int64  `gorm:"not null" json:"camp"`
	BindGUID         int64  `gorm:"not null" json:"bind_guid"`
	BuildingKind     string `gorm:"type:varchar(16);not null" json:"building_kind"`
	Generation       int64  `gorm:"not null;default:1" json:"generation"`
	OfferHash        string `gorm:"type:char(64);not null" json:"offer_hash"`
	InitialFrame     []byte `gorm:"type:longblob;not null" json:"initial_frame"`
	InitialHash      string `gorm:"type:char(64);not null" json:"initial_hash"`
	Frame            []byte `gorm:"type:longblob;not null" json:"frame"`
	FrameHash        string `gorm:"type:char(64);not null" json:"frame_hash"`
	PendingAttemptID string `gorm:"type:char(64);not null;default:''" json:"pending_attempt_id"`
	DefeatedAt       int64  `gorm:"not null;default:0" json:"defeated_at"`
	RecoverAt        int64  `gorm:"not null;default:0" json:"recover_at"`
	BuildingEvent    string `gorm:"type:varchar(32);not null;default:''" json:"building_event"`
	SourceHash       string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightGuardState) TableName() string { return "p13_union_fight_guard_states" }

package db

// P13UnionFightTowerState combines one immutable per-match source and its clock.
// A shot's sequence is allocated at launch; cancelled shots leave a sequence gap.
type P13UnionFightTowerState struct {
	PendingTargetLife         int64   `gorm:"not null;default:0" json:"pending_target_life,omitempty"`
	PendingActorGUID          string  `gorm:"type:varchar(128);not null;default:''" json:"pending_actor_guid,omitempty"`
	PendingActorPose          int64   `gorm:"not null;default:0" json:"pending_actor_pose,omitempty"`
	PendingFrozenActor        []byte  `gorm:"type:blob" json:"pending_frozen_actor,omitempty"`
	PendingFrozenActorHash    string  `gorm:"type:char(64);not null;default:''" json:"pending_frozen_actor_hash,omitempty"`
	PendingLaunchFrame        []byte  `gorm:"type:blob" json:"pending_launch_frame,omitempty"`
	PendingLaunchFrameHash    string  `gorm:"type:char(64);not null;default:''" json:"pending_launch_frame_hash,omitempty"`
	PendingOccupancyKind      string  `gorm:"type:varchar(16);not null;default:''" json:"pending_occupancy_kind,omitempty"`
	PendingOccupancyID        string  `gorm:"type:char(64);not null;default:''" json:"pending_occupancy_id,omitempty"`
	PendingOccupancyInputHash string  `gorm:"type:char(64);not null;default:''" json:"pending_occupancy_input_hash,omitempty"`
	PendingOccupancyDueAt     int64   `gorm:"not null;default:0" json:"pending_occupancy_due_at,omitempty"`
	PlayerAttack              int64   `gorm:"not null;default:0" json:"player_attack,omitempty"`
	PlayerDamageRule          string  `gorm:"type:varchar(64);not null;default:''" json:"player_damage_rule,omitempty"`
	MatchID                   string  `gorm:"primaryKey;type:char(64)" json:"match_id"`
	TowerGUID                 int64   `gorm:"primaryKey;autoIncrement:false" json:"tower_guid"`
	TowerID                   int64   `gorm:"not null" json:"tower_id"`
	GuardGUID                 int64   `gorm:"not null" json:"guard_guid"`
	Camp                      int64   `gorm:"not null" json:"camp"`
	X                         float32 `gorm:"not null" json:"x"`
	Y                         float32 `gorm:"not null" json:"y"`
	Z                         float32 `gorm:"not null" json:"z"`
	Range                     float64 `gorm:"not null" json:"range"`
	HPPercent                 int64   `gorm:"not null" json:"hp_percent"`
	CooldownMillis            int64   `gorm:"not null" json:"cooldown_millis"`
	ReleaseMillis             int64   `gorm:"not null" json:"release_millis"`
	AnimationMillis           int64   `gorm:"not null" json:"animation_millis"`
	ReviveSeconds             string  `gorm:"type:varchar(128);not null" json:"revive_seconds"`
	MatchHash                 string  `gorm:"type:char(64);not null" json:"match_hash"`
	Version                   string  `gorm:"type:varchar(64);not null" json:"version"`
	PublishedAt               int64   `gorm:"not null" json:"published_at"`
	Source                    string  `gorm:"type:text;not null" json:"source"`
	SourceHash                string  `gorm:"type:char(64);not null" json:"source_hash"`
	Active                    bool    `gorm:"not null;default:true" json:"active"`
	NextFireAt                int64   `gorm:"not null;index:p13_tower_due,priority:1" json:"next_fire_at"`
	PendingReleaseAt          int64   `gorm:"not null;default:0" json:"pending_release_at"`
	PendingTargetGUID         int64   `gorm:"not null;default:0" json:"pending_target_guid"`
	PendingTargetKind         string  `gorm:"type:varchar(8);not null;default:''" json:"pending_target_kind"`
	Sequence                  int64   `gorm:"not null;default:0" json:"sequence"`
	LastResolvedAt            int64   `gorm:"not null;default:0" json:"last_resolved_at"`
	StateHash                 string  `gorm:"type:char(64);not null" json:"state_hash"`
}

func (P13UnionFightTowerState) TableName() string { return "p13_union_fight_tower_states" }

package db

// P13UnionFightTowerHit is an immutable environmental HP/death journal.
// Preserve it after replay payload expiry; it proves no player received a kill.
type P13UnionFightTowerHit struct {
	ReleaseFrame             []byte  `gorm:"type:blob" json:"release_frame,omitempty"`
	ReleaseFrameHash         string  `gorm:"type:char(64);not null;default:''" json:"release_frame_hash,omitempty"`
	LaunchOccupancyKind      string  `gorm:"type:varchar(16);not null;default:''" json:"launch_occupancy_kind,omitempty"`
	LaunchOccupancyID        string  `gorm:"type:char(64);not null;default:''" json:"launch_occupancy_id,omitempty"`
	LaunchOccupancyInputHash string  `gorm:"type:char(64);not null;default:''" json:"launch_occupancy_input_hash,omitempty"`
	LaunchOccupancyDueAt     int64   `gorm:"not null;default:0" json:"launch_occupancy_due_at,omitempty"`
	CoalescedInto            string  `gorm:"type:char(64);not null;default:''" json:"coalesced_into,omitempty"`
	CancelFrame              []byte  `gorm:"type:blob" json:"cancel_frame,omitempty"`
	CancelFrameHash          string  `gorm:"type:char(64);not null;default:''" json:"cancel_frame_hash,omitempty"`
	CancelLeftAt             int64   `gorm:"not null;default:0" json:"cancel_left_at,omitempty"`
	Status                   string  `gorm:"type:varchar(16);not null;default:'';index:p13_tower_pending,priority:3;index:p13_tower_occupancy,priority:6" json:"status,omitempty"`
	CutoffAt                 int64   `gorm:"not null;default:0" json:"cutoff_at,omitempty"`
	HitAt                    int64   `gorm:"not null;default:0" json:"hit_at,omitempty"`
	CancelReason             string  `gorm:"type:varchar(64);not null;default:''" json:"cancel_reason,omitempty"`
	ActorGUID                string  `gorm:"type:varchar(128);not null;default:''" json:"actor_guid,omitempty"`
	ActorPose                int64   `gorm:"not null;default:0" json:"actor_pose,omitempty"`
	LifeOrdinal              int64   `gorm:"not null;default:0;index:p13_tower_pending,priority:4" json:"life_ordinal,omitempty"`
	FrozenActor              []byte  `gorm:"type:blob" json:"frozen_actor,omitempty"`
	FrozenActorHash          string  `gorm:"type:char(64);not null;default:''" json:"frozen_actor_hash,omitempty"`
	CaptureFrame             []byte  `gorm:"type:blob" json:"capture_frame,omitempty"`
	CaptureFrameHash         string  `gorm:"type:char(64);not null;default:''" json:"capture_frame_hash,omitempty"`
	Damage                   float64 `gorm:"not null;default:0" json:"damage,omitempty"`
	OccupancyKind            string  `gorm:"type:varchar(16);not null;default:'';index:p13_tower_occupancy,priority:3" json:"occupancy_kind,omitempty"`
	OccupancyID              string  `gorm:"type:char(64);not null;default:'';index:p13_tower_pending,priority:5;index:p13_tower_occupancy,priority:4" json:"occupancy_id,omitempty"`
	OccupancyInputHash       string  `gorm:"type:char(64);not null;default:''" json:"occupancy_input_hash,omitempty"`
	OccupancyDueAt           int64   `gorm:"not null;default:0" json:"occupancy_due_at,omitempty"`
	ImmutableHash            string  `gorm:"type:char(64);not null;default:''" json:"immutable_hash,omitempty"`
	MatchID                  string  `gorm:"primaryKey;type:char(64);index:p13_tower_occupancy,priority:1;index:p13_tower_pending,priority:1;index:p13_tower_target,priority:1;index:p13_tower_death,priority:1" json:"match_id"`
	TowerGUID                int64   `gorm:"primaryKey;autoIncrement:false;index:p13_tower_occupancy,priority:5" json:"tower_guid"`
	Sequence                 int64   `gorm:"primaryKey;autoIncrement:false" json:"sequence"`
	TargetKind               string  `gorm:"type:varchar(8);not null;index:p13_tower_target,priority:2" json:"target_kind"`
	TargetGUID               int64   `gorm:"not null;index:p13_tower_target,priority:3;index:p13_tower_pending,priority:2;index:p13_tower_occupancy,priority:2" json:"target_guid"`
	DueAt                    int64   `gorm:"not null;index:p13_tower_target,priority:4;index:p13_tower_death,priority:3" json:"due_at"`
	AppliedAt                int64   `gorm:"not null" json:"applied_at"`
	X                        float32 `gorm:"not null" json:"x"`
	Y                        float32 `gorm:"not null" json:"y"`
	Z                        float32 `gorm:"not null" json:"z"`
	Before                   []byte  `gorm:"type:blob;not null" json:"before"`
	After                    []byte  `gorm:"type:blob;not null" json:"after"`
	BeforeHash               string  `gorm:"type:char(64);not null" json:"before_hash"`
	AfterHash                string  `gorm:"type:char(64);not null" json:"after_hash"`
	DeadPlayerID             int64   `gorm:"not null;default:0;index:p13_tower_death,priority:2" json:"dead_player_id"`
	ReviveSeconds            int64   `gorm:"not null;default:0" json:"revive_seconds"`
	ReviveAt                 int64   `gorm:"not null;default:0" json:"revive_at"`
	TowerSourceHash          string  `gorm:"type:char(64);not null" json:"tower_source_hash"`
	HitHash                  string  `gorm:"type:char(64);not null" json:"hit_hash"`
}

func (P13UnionFightTowerHit) TableName() string { return "p13_union_fight_tower_hits" }

package db

// P13UnionFightGuardBattleAttempt preserves the independent guard lifecycle; existing audit records remain untouched.
type P13UnionFightGuardBattleAttempt struct {
	BuildingKind        string `gorm:"type:varchar(16);not null;default:'';index:p13_guard_battle_base_barrier,priority:3" json:"building_kind,omitempty"`
	BuildingAppliedAt   int64  `gorm:"not null;default:0" json:"building_applied_at,omitempty"`
	BuildingProof       []byte `gorm:"type:blob" json:"building_proof,omitempty"`
	GuardGeneration     int64  `gorm:"not null" json:"guard_generation"`
	ID                  string `gorm:"primaryKey;type:char(64);index:p13_guard_battle_compute,priority:6;index:p13_guard_battle_due,priority:3;index:p13_guard_battle_death,priority:6,sort:desc" json:"id"`
	MatchID             string `gorm:"type:char(64);not null;index:p13_guard_battle_base_barrier,priority:1;index:p13_guard_battle_guard,priority:1;index:p13_guard_battle_death,priority:1" json:"match_id"`
	PlayerID            int64  `gorm:"not null;index:p13_guard_battle_client,priority:1;index:p13_guard_battle_death,priority:2;uniqueIndex:p13_guard_battle_stamp,priority:1" json:"player_id"`
	GuardGUID           int64  `gorm:"not null;index:p13_guard_battle_guard,priority:2" json:"guard_guid"`
	Stamp               int64  `gorm:"not null;uniqueIndex:p13_guard_battle_stamp,priority:2" json:"stamp"`
	ClientStamp         int64  `gorm:"not null;index:p13_guard_battle_client,priority:2" json:"client_stamp"`
	Status              string `gorm:"type:varchar(16);not null;index:p13_guard_battle_base_barrier,priority:2;index:p13_guard_battle_compute,priority:1;index:p13_guard_battle_due,priority:1;index:p13_guard_battle_guard,priority:3;index:p13_guard_battle_death,priority:3" json:"status"`
	CalculationStage    string `gorm:"type:varchar(16);not null;index:p13_guard_battle_compute,priority:2" json:"calculation_stage"`
	NextCalculationAt   int64  `gorm:"not null;default:0;index:p13_guard_battle_compute,priority:4" json:"next_calculation_at"`
	CalculationFailures int64  `gorm:"not null;default:0" json:"calculation_failures"`
	Quarantined         bool   `gorm:"not null;default:false;index:p13_guard_battle_compute,priority:3" json:"-"`
	P6RecordID          string `gorm:"type:char(64);not null;uniqueIndex:p13_guard_battle_p6" json:"p6_record_id"`
	FightID             string `gorm:"type:varchar(128);not null" json:"fight_id"`
	InputHash           string `gorm:"type:char(64);not null" json:"input_hash"`
	OfferHash           string `gorm:"type:char(64);not null" json:"offer_hash"`
	RequestHash         string `gorm:"type:char(64);not null" json:"request_hash"`
	Rules               []byte `gorm:"type:longblob;not null" json:"rules"`
	PlayerBefore        []byte `gorm:"type:longblob;not null" json:"player_before"`
	GuardBefore         []byte `gorm:"type:longblob;not null" json:"guard_before"`
	ReservationHash     string `gorm:"type:char(64);not null" json:"reservation_hash"`
	Terminal            []byte `gorm:"type:longblob" json:"terminal"`
	TerminalHash        string `gorm:"type:char(64);not null;default:''" json:"terminal_hash"`
	PlayerAfter         []byte `gorm:"type:longblob" json:"player_after"`
	GuardAfter          []byte `gorm:"type:longblob" json:"guard_after"`
	PlayerDied          bool   `gorm:"not null;default:false;index:p13_guard_battle_death,priority:4" json:"player_died"`
	GuardDefeated       bool   `gorm:"not null;default:false" json:"guard_defeated"`
	ReviveSeconds       int64  `gorm:"not null;default:0" json:"revive_seconds"`
	BuildingEvent       string `gorm:"type:varchar(32);not null;default:''" json:"building_event"`
	CreatedAt           int64  `gorm:"not null;autoCreateTime:false;index:p13_guard_battle_compute,priority:5;index:p13_guard_battle_death,priority:5,sort:desc" json:"created_at"`
	DueAt               int64  `gorm:"not null;index:p13_guard_battle_base_barrier,priority:4;index:p13_guard_battle_due,priority:2" json:"due_at"`
	SettledAt           int64  `gorm:"not null;default:0" json:"settled_at"`
	SourceHash          string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightGuardBattleAttempt) TableName() string {
	return "p13_union_fight_guard_battle_attempts"
}
